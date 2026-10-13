// signal-receiver bridges Prometheus Alertmanager webhook payloads to the
// JSON-RPC API of signal-cli-rest-api running in MODE=json-rpc (port 8081).
//
// Env:
//
//	SIGNAL_RPC_URL  base URL of signal-cli-rest-api json-rpc (default http://localhost:8081)
//	SIGNAL_NUMBERS  comma-separated recipient numbers
//	LISTEN_ADDR     listen address, default :8080
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const maxMessageLen = 3500

// amPayload mirrors the parts of the Alertmanager webhook body we render.
type amPayload struct {
	Status      string `json:"status"`
	Alerts      []amAlert
	GroupLabels map[string]string `json:"groupLabels"`
	Receiver    string            `json:"receiver"`
}

type amAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StopsAt     string            `json:"endsAt"`
	StartsAt    string            `json:"startsAt"`
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func render(p *amPayload) string {
	icon := "🔥"
	if p.Status != "firing" {
		icon = "✅"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s alertmanager %s", icon, p.Status)
	if len(p.Alerts) > 0 {
		if sev := p.Alerts[0].Labels["severity"]; sev != "" {
			fmt.Fprintf(&b, " [%s]", sev)
		}
	}
	b.WriteString("\n")
	for _, a := range p.Alerts {
		name := a.Labels["alertname"]
		if name == "" {
			name = "(unlabeled)"
		}
		if a.Status == "resolved" {
			fmt.Fprintf(&b, "RESOLVED %s\n", name)
		} else {
			fmt.Fprintf(&b, "FIRING %s\n", name)
		}
		for _, k := range []string{"job", "instance", "pod", "node", "container", "namespace"} {
			if v := a.Labels[k]; v != "" {
				fmt.Fprintf(&b, "  %s=%s\n", k, v)
			}
		}
		for _, k := range []string{"summary", "description"} {
			if v := a.Annotations[k]; v != "" {
				fmt.Fprintf(&b, "  %s: %s\n", k, strings.TrimSpace(v))
			}
		}
	}
	msg := strings.TrimRight(b.String(), "\n")
	if len(msg) > maxMessageLen {
		msg = msg[:maxMessageLen] + "\n…(truncated)"
	}
	return msg
}

func send(recipients []string, msg string) error {
	body, err := json.Marshal(map[string]any{
		"method":   "send",
		"params":   map[string]any{"recipients": recipients, "message": msg},
		"jsonrpc":  "2.0",
		"id":       1,
	})
	if err != nil {
		return err
	}
	url := os.Getenv("SIGNAL_RPC_URL")
	if url == "" {
		url = "http://localhost:8081"
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Post(
		strings.TrimRight(url, "/")+"/api/v1/rpc",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc: HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var rpc struct {
		Error  *struct{ Message string } `json:"error"`
		Result struct {
			Results []struct {
				Type string `json:"type"`
			} `json:"results"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &rpc); err != nil {
		return fmt.Errorf("rpc: bad response: %s", string(raw))
	}
	if rpc.Error != nil {
		return fmt.Errorf("rpc error: %s", rpc.Error.Message)
	}
	for _, r := range rpc.Result.Results {
		if r.Type != "SUCCESS" {
			return fmt.Errorf("rpc: recipient send failed: %s", r.Type)
		}
	}
	return nil
}

func handler(numbers []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprintln(w, "ok")
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var p amPayload
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&p); err != nil {
			http.Error(w, "bad payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		msg := render(&p)
		log.Printf("alerts=%d status=%s: %s", len(p.Alerts), p.Status, strings.ReplaceAll(msg, "\n", " | "))
		if err := send(numbers, msg); err != nil {
			// non-2xx makes alertmanager retry this notification.
			log.Printf("send failed: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		fmt.Fprintln(w, "sent")
	}
}

func main() {
	numbers := []string{}
	for _, n := range strings.Split(env("SIGNAL_NUMBERS", ""), ",") {
		if n = strings.TrimSpace(n); n != "" {
			numbers = append(numbers, n)
		}
	}
	if len(numbers) == 0 {
		log.Fatal("SIGNAL_NUMBERS is empty")
	}
	addr := env("LISTEN_ADDR", ":8080")
	rpcURL := env("SIGNAL_RPC_URL", "http://localhost:8081")
	log.Printf("listening on %s, %d recipients, via %s", addr, len(numbers), rpcURL)
	log.Fatal(http.ListenAndServe(addr, handler(numbers)))
}
