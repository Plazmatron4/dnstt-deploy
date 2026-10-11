package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	addr    = flag.String("addr", "0.0.0.0:8080", "listen address for the UI")
	pass    = flag.String("pass", "", "password (or set UI_PASS); generated if empty")
	root    = flag.String("root", ".", "directory to browse")
	domain  = flag.String("domain", "", "dns tunnel domain, for the NS hint")
	pubfile = flag.String("pub", "keys/server.pub", "dnstt public key file")
	logfile = flag.String("log", "logs/dnstt.log", "dnstt log to tail")
	pidfile = flag.String("pid", "logs/dnstt.pid", "dnstt pid file")
)

const page = `<!doctype html><html><head><meta charset=utf-8>
<meta name=viewport content="width=device-width,initial-scale=1">
<title>%s</title><style>
:root{--bg:#12141a;--fg:#e6e6e6;--dim:#8b93a7;--acc:#6cf;--warn:#f96;--ok:#6f9}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.5 ui-monospace,Menlo,Consolas,monospace}
header{padding:14px 18px;border-bottom:1px solid #262a35;display:flex;gap:16px;align-items:center;flex-wrap:wrap}
header b{color:var(--acc)}
nav a{color:var(--dim);text-decoration:none;margin-right:14px}
nav a:hover{color:var(--acc)}
main{padding:18px;max-width:1100px}
pre{background:#0c0e13;padding:12px;border-radius:6px;overflow:auto;border:1px solid #262a35}
table{border-collapse:collapse;width:100%%}
td,th{padding:6px 10px;border-bottom:1px solid #1d2130;text-align:left;vertical-align:top}
th{color:var(--dim);font-weight:400}
a{color:var(--acc)}
.ok{color:var(--ok)}.bad{color:var(--warn)}
input,button{font:inherit;background:#0c0e13;color:var(--fg);border:1px solid #333a4a;border-radius:5px;padding:7px 10px}
button{cursor:pointer}button:hover{border-color:var(--acc)}
.row{display:flex;gap:10px;flex-wrap:wrap;align-items:center;margin:10px 0}
small{color:var(--dim)}
</style></head><body>
<header><b>dnstt</b>
<nav><a href="/">status</a><a href="/files/">files</a><a href="/sh">shell</a></nav>
<small>%s</small></header><main>%s</main></body></html>`

func wrap(title, body string) string {
	return fmt.Sprintf(page, html.EscapeString(title), time.Now().Format("2006-01-02 15:04:05"), body)
}

func dash(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	// tunnel process
	pid := ""
	if d, err := os.ReadFile(*pidfile); err == nil {
		pid = strings.TrimSpace(string(d))
	}
	alive := false
	if pid != "" {
		if _, err := os.Stat("/proc/" + pid); err == nil {
			alive = true
		}
	}
	status := `<span class="bad">not running</span>`
	if alive {
		status = `<span class="ok">running (pid ` + pid + `)</span>`
	}
	fmt.Fprintf(&b, "<h3>dns tunnel</h3><table>")
	fmt.Fprintf(&b, "<tr><th>server</th><td>%s</td></tr>", status)
	fmt.Fprintf(&b, "<tr><th>domain</th><td>%s</td></tr>", html.EscapeString(*domain))
	if d, err := os.ReadFile(*pubfile); err == nil {
		fmt.Fprintf(&b, "<tr><th>public key</th><td><code>%s</code></td></tr>", html.EscapeString(strings.TrimSpace(string(d))))
	} else {
		fmt.Fprintf(&b, "<tr><th>public key</th><td class=bad>not generated yet</td></tr>")
	}
	h, _ := os.Hostname()
	fmt.Fprintf(&b, "<tr><th>host</th><td>%s</td></tr>", html.EscapeString(h))
	fmt.Fprintf(&b, "<tr><th>listen</th><td><pre>%s</pre></td></tr>", html.EscapeString(sockets()))
	fmt.Fprintf(&b, "<tr><th>uptime</th><td>%s</td></tr>", html.EscapeString(uptime()))
	fmt.Fprintf(&b, "<tr><th>disk</th><td><pre>%s</pre></td></tr>", html.EscapeString(disk()))
	b.WriteString("</table>")

	if *logfile != "" {
		if d, err := os.ReadFile(*logfile); err == nil {
			lines := strings.Split(strings.TrimRight(string(d), "\n"), "\n")
			if len(lines) > 40 {
				lines = lines[len(lines)-40:]
			}
			fmt.Fprintf(&b, "<h3>tunnel log <small>(last %d lines)</small></h3><pre>%s</pre>",
				len(lines), html.EscapeString(strings.Join(lines, "\n")))
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, wrap("status", b.String()))
}

func sockets() string {
	out, err := exec.Command("sh", "-c", "ss -lunp 2>/dev/null | head -20 || netstat -lun 2>/dev/null | head -20").Output()
	if err != nil || len(out) == 0 {
		return "(ss/netstat unavailable)"
	}
	return string(out)
}

func uptime() string {
	d, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "?"
	}
	f := strings.Fields(string(d))
	if len(f) == 0 {
		return "?"
	}
	s, _ := strconv.ParseFloat(f[0], 64)
	return (time.Duration(s) * time.Second).String()
}

func disk() string {
	out, _ := exec.Command("df", "-h", *root).Output()
	return string(out)
}

// safePath resolves a user-supplied path under root, refusing traversal.
func safePath(p string) (string, error) {
	abs, err := filepath.Abs(*root)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean("/" + p)
	full := filepath.Join(abs, clean)
	if full != abs && !strings.HasPrefix(full, abs+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return full, nil
}

func files(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/files")
	if rel == "" {
		rel = "/"
	}
	full, err := safePath(rel)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	fi, err := os.Stat(full)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	var b strings.Builder
	if fi.IsDir() {
		ents, err := os.ReadDir(full)
		if err != nil {
			http.Error(w, err.Error(), 403)
			return
		}
		sort.Slice(ents, func(i, j int) bool {
			if ents[i].IsDir() != ents[j].IsDir() {
				return ents[i].IsDir()
			}
			return ents[i].Name() < ents[j].Name()
		})
		fmt.Fprintf(&b, `<h3>%s</h3>`, html.EscapeString(rel))
		fmt.Fprintf(&b, `<div class=row><form method=post action="/upload" enctype="multipart/form-data">`+
			`<input type=hidden name=dir value="%s"><input type=file name=f multiple>`+
			`<button>upload</button></form>`+
			`<form method=post action="/mkdir"><input type=hidden name=dir value="%s">`+
			`<input name=name placeholder="new folder"><button>mkdir</button></form></div>`,
			html.EscapeString(rel), html.EscapeString(rel))
		b.WriteString("<table><tr><th>name</th><th>size</th><th>modified</th><th></th></tr>")
		if rel != "/" {
			up := filepath.Dir(strings.TrimRight(rel, "/"))
			fmt.Fprintf(&b, `<tr><td><a href="/files%s">../</a></td><td></td><td></td><td></td></tr>`, html.EscapeString(up))
		}
		for _, e := range ents {
			name := e.Name()
			p := filepath.Join(rel, name)
			link := "/files" + p
			if !e.IsDir() {
				link = "/raw" + p
			}
			info, _ := e.Info()
			sz, mod := "", ""
			if info != nil {
				sz = humanSize(info.Size())
				mod = info.ModTime().Format("01-02 15:04")
			}
			if e.IsDir() {
				sz = "dir"
			}
			fmt.Fprintf(&b, `<tr><td><a href="%s">%s</a></td><td>%s</td><td>%s</td>`+
				`<td><form method=post action="/rm" onsubmit="return confirm('delete %s?')">`+
				`<input type=hidden name=path value="%s"><button>del</button></form></td></tr>`,
				html.EscapeString(link), html.EscapeString(name), sz, mod,
				html.EscapeString(name), html.EscapeString(p))
		}
		b.WriteString("</table>")
	} else {
		fmt.Fprintf(&b, `<h3>%s</h3><p>%s</p><pre>%s</pre>`,
			html.EscapeString(rel), humanSize(fi.Size()), html.EscapeString(head(full, 8192)))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, wrap("files", b.String()))
}

func head(p string, n int) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, n)
	m, _ := io.ReadFull(f, buf)
	s := string(buf[:m])
	if !isText(s) {
		return "(binary)"
	}
	return s
}

func isText(s string) bool {
	for _, r := range s {
		if r == 0 {
			return false
		}
	}
	return true
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d", n)
}

func raw(w http.ResponseWriter, r *http.Request) {
	full, err := safePath(strings.TrimPrefix(r.URL.Path, "/raw"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.ServeFile(w, r, full)
}

func upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 30); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	dir, err := safePath(r.FormValue("dir"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for _, fh := range r.MultipartForm.File["f"] {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		dst, err := os.Create(filepath.Join(dir, filepath.Base(fh.Filename)))
		if err == nil {
			io.Copy(dst, f)
			dst.Close()
		}
		f.Close()
	}
	http.Redirect(w, r, "/files"+r.FormValue("dir"), 303)
}

func mkdir(w http.ResponseWriter, r *http.Request) {
	dir, err := safePath(filepath.Join(r.FormValue("dir"), r.FormValue("name")))
	if err == nil {
		os.MkdirAll(dir, 0o755)
	}
	http.Redirect(w, r, "/files"+r.FormValue("dir"), 303)
}

func rm(w http.ResponseWriter, r *http.Request) {
	full, err := safePath(r.FormValue("path"))
	if err == nil {
		os.RemoveAll(full)
	}
	http.Redirect(w, r, "/files"+filepath.Dir(r.FormValue("path")), 303)
}

// cmdValue reads the shell command from ?cmd= or the POST body.
//
// r.FormValue() cannot be used here: Go 1.17+ makes url.ParseQuery reject a
// body containing a literal ';', so any command with a semicolon in it comes
// back empty. Read the body ourselves and decode it leniently.
func cmdValue(r *http.Request) string {
	if v := r.URL.Query().Get("cmd"); v != "" {
		return v
	}
	if r.Method != http.MethodPost {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	s := strings.TrimPrefix(string(b), "cmd=")
	if i := strings.IndexByte(s, '&'); i >= 0 {
		s = s[:i]
	}
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return strings.ReplaceAll(s, "+", " ")
}

func shell(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	cmd := cmdValue(r)
	b.WriteString(`<h3>shell</h3><form method=post action="/sh" class=row>` +
		`<input name=cmd style="flex:1;min-width:260px" value="` + html.EscapeString(cmd) + `" autofocus>` +
		`<button>run</button></form>`)
	if cmd != "" {
		c := exec.Command("sh", "-c", cmd)
		c.Dir = *root
		out, _ := c.CombinedOutput()
		if len(out) > 200000 {
			out = out[:200000]
		}
		fmt.Fprintf(&b, "<pre>%s</pre>", html.EscapeString(string(out)))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, wrap("shell", b.String()))
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(p), []byte(*pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="dnstt"`)
			http.Error(w, "auth required", 401)
			return
		}
		next(w, r)
	}
}

func main() {
	flag.Parse()
	if *pass == "" {
		*pass = os.Getenv("UI_PASS")
	}
	if *pass == "" {
		b := make([]byte, 9)
		rand.Read(b)
		*pass = hex.EncodeToString(b)
		fmt.Printf("\n  UI password: %s\n\n", *pass)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", auth(dash))
	mux.HandleFunc("/files/", auth(files))
	mux.HandleFunc("/raw/", auth(raw))
	mux.HandleFunc("/upload", auth(upload))
	mux.HandleFunc("/mkdir", auth(mkdir))
	mux.HandleFunc("/rm", auth(rm))
	mux.HandleFunc("/sh", auth(shell))
	log.Printf("ui on http://%s  (user: any, password: %s)", *addr, *pass)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
