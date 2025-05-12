package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Records     map[string]string `yaml:"records"`
	FallbackDNS string            `yaml:"fallback_dns"`
}

var (
	config     Config
	configLock sync.RWMutex
	configFile = "/app/config/config.yaml" // Path inside the container
)

func loadConfig() error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var newConfig Config
	err = yaml.Unmarshal(data, &newConfig)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	configLock.Lock()
	defer configLock.Unlock()
	config = newConfig
	log.Println("Configuration loaded/reloaded")
	log.Printf("Records: %v", config.Records)
	log.Printf("Fallback DNS: %s", config.FallbackDNS)
	return nil
}

func watchConfig() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	defer watcher.Close()

	err = watcher.Add(configFile)
	if err != nil {
		// Fallback for environments where the exact file path might be a symlink or mount point
		// Try watching the directory instead.
		log.Printf("Failed to watch config file directly (%s): %v. Watching directory /app/config/ instead.", configFile, err)
		err = watcher.Add("/app/config/")
		if err != nil {
			log.Fatalf("Failed to watch config directory: %v", err)
		}
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			log.Printf("Config file event: %s", event)
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				log.Println("Config file modified, attempting to reload...")
				if err := loadConfig(); err != nil {
					log.Printf("Error reloading config: %v. Ignoring changes.", err)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Error watching config file: %v", err)
		}
	}
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		log.Printf("Received query for %s, type %s", q.Name, dns.TypeToString[q.Qtype])
		if q.Qtype == dns.TypeA {
			configLock.RLock()
			ip, exists := config.Records[strings.ToLower(strings.TrimSuffix(q.Name, "."))]
			fallbackDNS := config.FallbackDNS
			configLock.RUnlock()

			if exists {
				log.Printf("Found record for %s -> %s in config", q.Name, ip)
				rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
				if err == nil {
					msg.Answer = append(msg.Answer, rr)
				} else {
					log.Printf("Error creating A record for %s: %v", q.Name, err)
					msg.Rcode = dns.RcodeServerFailure
				}
			} else {
				log.Printf("No record for %s in config, relaying to %s", q.Name, fallbackDNS)
				if fallbackDNS == "" {
					log.Printf("Fallback DNS not configured, returning NXDOMAIN for %s", q.Name)
					msg.Rcode = dns.RcodeNameError // NXDOMAIN
				} else {
					// Relay to fallback DNS
					c := new(dns.Client)
					in, _, err := c.Exchange(r, fallbackDNS+":53") // Ensure port is specified
					if err != nil {
						log.Printf("Error relaying query for %s to %s: %v", q.Name, fallbackDNS, err)
						msg.Rcode = dns.RcodeServerFailure
					} else {
						msg = in
					}
				}
			}
		} else {
			log.Printf("Unsupported query type %s for %s, returning NotImp", dns.TypeToString[q.Qtype], q.Name)
			msg.Rcode = dns.RcodeNotImplemented
		}
	}

	err := w.WriteMsg(msg)
	if err != nil {
		log.Printf("Error writing DNS response: %v", err)
	}
}

func main() {
	// Initial load
	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load initial config: %v", err)
	}

	// Watch for config changes in a goroutine
	go watchConfig()

	// Attach handler function
	dns.HandleFunc(".", handleDNSRequest)

	// Start DNS server
	port := 53

	// Start UDP server
	go func() {
		serverUDP := &dns.Server{Addr: fmt.Sprintf(":%d", port), Net: "udp"}
		log.Printf("Starting UDP DNS server on port %d", port)
		err := serverUDP.ListenAndServe()
		if err != nil {
			log.Fatalf("Failed to start UDP server: %v", err)
		}
		defer serverUDP.Shutdown()
	}()

	// Start TCP server
	serverTCP := &dns.Server{Addr: fmt.Sprintf(":%d", port), Net: "tcp"}
	log.Printf("Starting TCP DNS server on port %d", port)
	err := serverTCP.ListenAndServe()
	if err != nil {
		log.Fatalf("Failed to start TCP server: %v", err)
	}
	defer serverTCP.Shutdown()
}
