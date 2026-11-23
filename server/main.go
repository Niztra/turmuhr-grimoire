// Turmuhr-Server: liefert die Grimoire-App im lokalen Netz aus und verteilt
// den öffentlichen Spielstand an die Handys der Spieler und die Befehle an die Soundanlage.
//
// Der Server kennt nur, was der Spielleiter ausdrücklich hochschickt: den öffentlichen Stand
// (Skript, Namen, Leben/Tod, Phase, Nominierungen) und je Spieler-Handy dessen eigene Daten
// (freigegebene Rolle, private Nachrichten). Das Grimoire selbst bleibt auf dem Gerät des Spielleiters.
// Alles liegt nur im Arbeitsspeicher und ist nach dem Beenden weg.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

var version = "dev"

const maxGuests = 80

type client struct {
	ch   chan []byte
	done chan struct{}
	once sync.Once
}

func (c *client) send(b []byte) {
	select {
	case c.ch <- b:
	default: // zu langsam: Verbindung schließen, der Browser verbindet sich selbst neu
		c.close()
	}
}
func (c *client) close() { c.once.Do(func() { close(c.done) }) }

type guest struct {
	Tok, Name, Kind string
	Code            string // Raum, dem dieses Handy beigetreten ist
	Bot             bool   // Simulation: Bot-Spieler ohne eigenes Gerät
	conns           map[*client]bool
	seen            time.Time
}

type hub struct {
	mu     sync.Mutex
	code   string // Raumcode, öffentlich (steht im Einladungslink)
	key    string // Schlüssel des Spielleiter-Geräts, geheim
	pub    []byte
	me     map[string][]byte // Gerätecode -> eigene Daten dieses Handys
	guests map[string]*guest
	hosts  map[*client]bool
	sstate []byte
}

var H = &hub{me: map[string][]byte{}, guests: map[string]*guest{}, hosts: map[*client]bool{}}

func msg(v any) []byte { b, _ := json.Marshal(v); return b }

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func (h *hub) isHost(key string) bool {
	return h.key != "" && key != "" && subtle.ConstantTimeCompare([]byte(h.key), []byte(key)) == 1
}

// Liste der Geräte, nur für den Spielleiter
func (h *hub) guestList() []byte {
	type g struct {
		Tok  string `json:"tok"`
		Name string `json:"name"`
		Kind string `json:"kind"`
		On   bool   `json:"on"`
		Bot  bool   `json:"bot,omitempty"`
	}
	l := []g{}
	for _, x := range h.guests {
		if x.Kind == "spieler" && (h.code == "" || x.Code != h.code) {
			continue // Handys aus einem anderen (alten) Raum gehören nicht in diese Liste
		}
		l = append(l, g{x.Tok, x.Name, x.Kind, len(x.conns) > 0 || x.Bot, x.Bot})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Name < l[j].Name })
	return msg(map[string]any{"k": "guests", "list": l})
}

func (h *hub) toHosts(b []byte) {
	for c := range h.hosts {
		c.send(b)
	}
}

func (h *hub) meFor(x *guest) []byte {
	if b, ok := h.me[x.Tok]; ok {
		return b
	}
	return msg(map[string]any{"k": "me", "seat": 0, "name": x.Name, "role": "", "msgs": []any{}})
}

func (h *hub) toGuest(x *guest, b []byte) {
	for c := range x.conns {
		c.send(b)
	}
}

func (h *hub) dropBots() {
	for t, x := range h.guests {
		if x.Bot {
			for c := range x.conns {
				c.close()
			}
			delete(h.guests, t)
		}
	}
}

func (h *hub) prune() {
	for t, x := range h.guests {
		if len(x.conns) == 0 && time.Since(x.seen) > 6*time.Hour {
			delete(h.guests, t)
		}
	}
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 512<<10)).Decode(v)
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	w.Write(msg(v))
}

// POST /api/host {code, key}: Der Spielleiter öffnet (oder übernimmt erneut) den Raum.
func apiHost(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code, Key string }
	if readJSON(r, &in) != nil || len(in.Key) < 16 || len(in.Code) < 4 {
		reply(w, 400, map[string]any{"error": "ungültig"})
		return
	}
	in.Code = clip(strings.ToUpper(in.Code), 12)
	h := H
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.key != "" && !h.isHost(in.Key) {
		// Übernahme nur, wenn kein anderes Spielleiter-Gerät verbunden ist, oder direkt vom Server-PC aus
		if len(h.hosts) > 0 && !isLoopback(r.RemoteAddr) {
			reply(w, 409, map[string]any{"error": "Ein anderes Spielleiter-Gerät ist gerade mit dem Server verbunden. Schließ die App dort, dann klappt es hier."})
			return
		}
		for c := range h.hosts {
			c.close()
		}
	}
	if h.code != in.Code { // neuer Raum: alte Daten weg, Handys des alten Raums erfahren es
		old := h.code
		h.code, h.pub, h.me = in.Code, nil, map[string][]byte{}
		h.dropBots()
		if old != "" {
			for _, x := range h.guests {
				if x.Kind == "spieler" {
					h.toGuest(x, msg(map[string]any{"k": "closed"}))
				}
			}
		}
	}
	h.key = in.Key
	reply(w, 200, map[string]any{"ok": true, "code": h.code})
}

// POST /api/close {key}: Raum schließen
func apiClose(w http.ResponseWriter, r *http.Request) {
	var in struct{ Key string }
	readJSON(r, &in)
	h := H
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.isHost(in.Key) {
		reply(w, 403, map[string]any{"error": "nicht erlaubt"})
		return
	}
	for _, x := range h.guests {
		if x.Kind == "spieler" {
			h.toGuest(x, msg(map[string]any{"k": "closed"}))
		}
	}
	h.code, h.key, h.pub, h.me = "", "", nil, map[string][]byte{}
	h.dropBots()
	h.toHosts(h.guestList())
	for c := range h.hosts {
		c.close()
	}
	reply(w, 200, map[string]any{"ok": true})
}

// POST /api/push {key, pub, me:{gerätecode: {...}}}: Spielleiter schickt den freigegebenen Stand
func apiPush(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string
		Pub json.RawMessage
		Me  map[string]json.RawMessage
	}
	if readJSON(r, &in) != nil {
		reply(w, 400, map[string]any{"error": "ungültig"})
		return
	}
	h := H
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.isHost(in.Key) {
		reply(w, 403, map[string]any{"error": "nicht erlaubt"})
		return
	}
	pubChanged := len(in.Pub) > 0 && string(in.Pub) != string(h.pub)
	if pubChanged {
		h.pub = append([]byte(nil), in.Pub...)
	}
	old := h.me
	h.me = map[string][]byte{}
	for t, b := range in.Me {
		if len(t) <= 40 && len(b) > 0 {
			h.me[t] = append([]byte(nil), b...)
		}
	}
	for _, x := range h.guests {
		if x.Kind != "spieler" || x.Code != h.code {
			continue
		}
		if pubChanged {
			h.toGuest(x, h.pub)
		}
		if string(h.me[x.Tok]) != string(old[x.Tok]) {
			h.toGuest(x, h.meFor(x))
		}
	}
	reply(w, 200, map[string]any{"ok": true})
}

// POST /api/bots {key, n}: Simulation, legt n Bot-Spieler an, die wie echte Handys behandelt werden
var botNames = []string{"Anna", "Ben", "Cem", "Dora", "Emil", "Fritz", "Gül", "Hanna", "Ida", "Jonas", "Karla", "Luca", "Mia", "Nico", "Olga"}

func apiBots(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string
		N   int
	}
	if readJSON(r, &in) != nil || in.N < 1 || in.N > 15 {
		reply(w, 400, map[string]any{"error": "ungültig"})
		return
	}
	h := H
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.isHost(in.Key) {
		reply(w, 403, map[string]any{"error": "nicht erlaubt"})
		return
	}
	h.dropBots()
	b := make([]byte, 12)
	for i := 0; i < in.N; i++ {
		rand.Read(b)
		tok := fmt.Sprintf("bot%x", b)
		h.guests[tok] = &guest{Tok: tok, Name: botNames[i] + " (Bot)", Kind: "spieler", Code: h.code, Bot: true, conns: map[*client]bool{}, seen: time.Now()}
	}
	h.toHosts(h.guestList())
	reply(w, 200, map[string]any{"ok": true})
}

// POST /api/snd {key, m}: Befehl an die Soundanlage(n)
func apiSnd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string
		M   json.RawMessage
	}
	if readJSON(r, &in) != nil {
		reply(w, 400, map[string]any{"error": "ungültig"})
		return
	}
	h := H
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.isHost(in.Key) {
		reply(w, 403, map[string]any{"error": "nicht erlaubt"})
		return
	}
	n := 0
	b := msg(map[string]any{"k": "snd", "m": in.M})
	for _, x := range h.guests {
		if x.Kind == "sound" && len(x.conns) > 0 {
			h.toGuest(x, b)
			n++
		}
	}
	reply(w, 200, map[string]any{"ok": true, "n": n})
}

// POST /api/sstate {tok, ...}: Soundanlage meldet, was gerade läuft
func apiSstate(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if readJSON(r, &in) != nil {
		reply(w, 400, map[string]any{"error": "ungültig"})
		return
	}
	tok, _ := in["tok"].(string)
	h := H
	h.mu.Lock()
	defer h.mu.Unlock()
	if x := h.guests[tok]; x == nil || x.Kind != "sound" {
		reply(w, 403, map[string]any{"error": "nicht erlaubt"})
		return
	}
	delete(in, "tok")
	in["k"] = "sstate"
	h.sstate = msg(in)
	h.toHosts(h.sstate)
	reply(w, 200, map[string]any{"ok": true})
}

// GET /api/events: Server-Sent Events. Spielleiter mit ?role=host&key=…, sonst ?role=spieler|sound&tok=…&name=…&code=…
func apiEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "kein Streaming", 500)
		return
	}
	q := r.URL.Query()
	role := q.Get("role")
	c := &client{ch: make(chan []byte, 64), done: make(chan struct{})}
	h := H
	var x *guest
	h.mu.Lock()
	switch role {
	case "host":
		if !h.isHost(q.Get("key")) {
			h.mu.Unlock()
			http.Error(w, "nicht erlaubt", 403)
			return
		}
		h.hosts[c] = true
		c.ch <- h.guestList()
		if h.sstate != nil {
			c.ch <- h.sstate
		}
	case "spieler", "sound":
		tok := q.Get("tok")
		if len(tok) < 16 || len(tok) > 40 {
			h.mu.Unlock()
			http.Error(w, "ungültig", 400)
			return
		}
		x = h.guests[tok]
		if x == nil {
			h.prune()
			if len(h.guests) >= maxGuests {
				h.mu.Unlock()
				http.Error(w, "zu viele Geräte", 503)
				return
			}
			x = &guest{Tok: tok, conns: map[*client]bool{}}
			h.guests[tok] = x
		}
		x.Kind, x.seen = role, time.Now()
		if n := clip(q.Get("name"), 30); n != "" {
			x.Name = n
		}
		x.conns[c] = true
		if role == "spieler" {
			code := strings.ToUpper(q.Get("code"))
			x.Code = code
			switch {
			case h.code == "":
				c.ch <- msg(map[string]any{"k": "wait"})
			case code != h.code:
				c.ch <- msg(map[string]any{"k": "closed"})
			default:
				if h.pub != nil {
					c.ch <- h.pub
				}
				c.ch <- h.meFor(x)
			}
		} else {
			c.ch <- msg(map[string]any{"k": "welcome", "kind": "sound"})
		}
		h.toHosts(h.guestList())
	default:
		h.mu.Unlock()
		http.Error(w, "ungültig", 400)
		return
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "retry: 2000\n\n")
	fl.Flush()
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	defer func() {
		h.mu.Lock()
		if x != nil {
			delete(x.conns, c)
			x.seen = time.Now()
			h.toHosts(h.guestList())
		} else {
			delete(h.hosts, c)
		}
		h.mu.Unlock()
	}()
	for {
		select {
		case b := <-c.ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-c.done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func lanURLs(port int) []string {
	var out, weak []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		n := strings.ToLower(i.Name)
		virtual := strings.HasPrefix(n, "docker") || strings.HasPrefix(n, "br-") || strings.HasPrefix(n, "virbr") || strings.HasPrefix(n, "veth") || strings.Contains(n, "vmware") || strings.Contains(n, "virtualbox") || strings.Contains(n, "vethernet") || strings.HasPrefix(n, "tailscale") || strings.HasPrefix(n, "zt") || strings.HasPrefix(n, "tun") || strings.HasPrefix(n, "wg")
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil || ip.To4() == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			u := fmt.Sprintf("http://%s:%d/", ip, port)
			if virtual {
				weak = append(weak, u)
			} else {
				out = append(out, u)
			}
		}
	}
	if len(out) == 0 {
		return weak
	}
	return out
}

// Kopplungscode: 6 Ziffern. Für die üblichen Heim-, Location- und Hotspot-Netze steckt die Adresse
// im Code selbst (so klappt es auch ohne Internet), sonst eine Zufallszahl, die nur über den Vermittlungsdienst gefunden wird.
//
//	1xxxxx = 192.168.a.b:8080 mit xxxxx = a*256+b
//	2000xx = 172.20.10.b:8080 (iPhone-Hotspot)
//	3xxxxx-9xxxxx = nur über das Internet
func pairCode(urls []string, port int) string {
	if len(urls) > 0 && port == 8080 {
		var a, b, c, d int
		if n, _ := fmt.Sscanf(urls[0], "http://%d.%d.%d.%d:", &a, &b, &c, &d); n == 4 {
			if a == 192 && b == 168 {
				return fmt.Sprintf("%06d", 100000+c*256+d)
			}
			if a == 172 && b == 20 && c == 10 {
				return fmt.Sprintf("%06d", 200000+d)
			}
		}
	}
	x := make([]byte, 4)
	rand.Read(x)
	r := (int(x[0])<<16 | int(x[1])<<8 | int(x[2])) % 700000
	return fmt.Sprintf("%06d", 300000+r)
}

func main() {
	initConsole()
	port := flag.Int("port", 8080, "Port")
	noBrowser := flag.Bool("kein-browser", false, "Browser beim Start nicht öffnen")
	flag.Parse()

	web, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(web))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/host", apiHost)
	mux.HandleFunc("POST /api/close", apiClose)
	mux.HandleFunc("POST /api/push", apiPush)
	mux.HandleFunc("POST /api/snd", apiSnd)
	mux.HandleFunc("POST /api/bots", apiBots)
	mux.HandleFunc("POST /api/sstate", apiSstate)
	mux.HandleFunc("GET /api/events", apiEvents)
	var urls []string
	var pair string
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"server": "turmuhr", "version": version, "urls": urls, "pair": pair})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "max-age=86400")
		}
		files.ServeHTTP(w, r)
	})

	var ln net.Listener
	var err error
	for p := *port; p < *port+20; p++ {
		if ln, err = net.Listen("tcp", fmt.Sprintf(":%d", p)); err == nil {
			*port = p
			break
		}
	}
	if err != nil {
		fmt.Println("Kein freier Port gefunden:", err)
		fmt.Println("Enter zum Beenden …")
		fmt.Scanln()
		os.Exit(1)
	}
	urls = lanURLs(*port)
	pair = pairCode(urls, *port)
	local := fmt.Sprintf("http://localhost:%d/", *port)

	fmt.Println("════════════════════════════════════════════════════")
	fmt.Println("  Turmuhr-Server", version, "läuft")
	fmt.Println("════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  KOPPLUNGSCODE:   ", pair[:3]+" "+pair[3:])
	fmt.Println("  Im Spielleiter-Handy unter Aufbau → „Mit dem PC koppeln“ eingeben.")
	fmt.Println()
	fmt.Println("  Auf diesem PC:  ", local)
	if len(urls) == 0 {
		fmt.Println("  Kein Netzwerk gefunden. Verbinde den PC mit dem WLAN oder Hotspot.")
	} else {
		fmt.Println("  Für Handys im selben WLAN/Hotspot:")
		for _, u := range urls {
			fmt.Println("      ", u)
		}
	}
	fmt.Println()
	fmt.Println("  Der Browser öffnet sich als Soundanlage und zeigt den Code groß an.")
	fmt.Println("  Fragt die Firewall nach: Zugriff im privaten Netzwerk erlauben.")
	fmt.Println("  Zum Beenden dieses Fenster schließen (oder Strg+C).")
	fmt.Println()

	if !*noBrowser {
		go func() { time.Sleep(600 * time.Millisecond); openBrowser(local + "#start") }()
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.Serve(ln))
}
