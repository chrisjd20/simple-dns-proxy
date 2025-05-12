package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Port      int    `yaml:"port"`
	Interface string `yaml:"interface"`
}

type Config struct {
	Records     map[string]string `yaml:"records"`
	FallbackDNS string            `yaml:"fallback_dns"`
	Server      struct {
		UDP ServerConfig `yaml:"udp"`
		TCP ServerConfig `yaml:"tcp"`
	} `yaml:"server"`
}

var (
	config            Config
	configLock        sync.RWMutex
	configFile        string                      // Will be set in init()
	defaultConfigPath = "/app/config/config.yaml" // Default path inside the container
)

// init finds and sets the config file path
func init() {
	// Check if the default config path exists
	if _, err := os.Stat(defaultConfigPath); err == nil {
		configFile = defaultConfigPath
		log.Printf("Found config file at default path: %s", configFile)
		return
	}

	// Check if config file exists in the current working directory
	pwd, err := os.Getwd()
	if err != nil {
		log.Printf("Warning: Failed to get current working directory: %v", err)
	} else {
		localConfigPath := filepath.Join(pwd, "config.yaml")
		if _, err := os.Stat(localConfigPath); err == nil {
			configFile = localConfigPath
			log.Printf("Found config file in current directory: %s", configFile)
			return
		}
	}

	// If config file is not found, default to the container path
	// This might fail later, but we'll handle that in loadConfig
	configFile = defaultConfigPath
	log.Printf("No config file found, will try to load from: %s", configFile)
}

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

	// Apply default values for server if not specified
	if config.Server.UDP.Port <= 0 {
		config.Server.UDP.Port = 53
	}
	if config.Server.TCP.Port <= 0 {
		config.Server.TCP.Port = 53
	}

	log.Println("Configuration loaded/reloaded")
	log.Printf("Records: %v", config.Records)
	log.Printf("Fallback DNS: %s", config.FallbackDNS)

	// Log server configuration
	log.Printf("UDP Server: enabled=%v, port=%d, interface=%q",
		config.Server.UDP.Enabled, config.Server.UDP.Port, config.Server.UDP.Interface)
	log.Printf("TCP Server: enabled=%v, port=%d, interface=%q",
		config.Server.TCP.Enabled, config.Server.TCP.Port, config.Server.TCP.Interface)

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
		configDir := filepath.Dir(configFile)
		log.Printf("Failed to watch config file directly (%s): %v. Watching directory %s instead.", configFile, err, configDir)
		err = watcher.Add(configDir)
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
		// Get the fallback DNS server for potential relaying
		configLock.RLock()
		fallbackDNS := config.FallbackDNS
		configLock.RUnlock()

		// For A records, check if we have a match in our config first
		if q.Qtype == dns.TypeA {
			configLock.RLock()
			ip, exists := config.Records[strings.ToLower(strings.TrimSuffix(q.Name, "."))]
			configLock.RUnlock()

			if exists {
				log.Printf("Found A record for %s -> %s in config", q.Name, ip)
				rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
				if err == nil {
					msg.Answer = append(msg.Answer, rr)
					continue // Process next question
				} else {
					log.Printf("Error creating A record for %s: %v", q.Name, err)
					msg.Rcode = dns.RcodeServerFailure
					continue // Process next question
				}
			}
		}

		// If we reach here, either:
		// 1. It's a non-A record query
		// 2. It's an A record query but not in our config
		// In both cases, relay to the fallback DNS if configured

		log.Printf("Relaying %s query for %s to fallback DNS %s",
			dns.TypeToString[q.Qtype], q.Name, fallbackDNS)

		if fallbackDNS == "" {
			log.Printf("Fallback DNS not configured, returning NXDOMAIN for %s", q.Name)
			msg.Rcode = dns.RcodeNameError // NXDOMAIN
		} else {
			// Relay to fallback DNS
			c := new(dns.Client)
			c.Net = w.RemoteAddr().Network()               // Use same protocol (UDP/TCP) as the client
			in, _, err := c.Exchange(r, fallbackDNS+":53") // Ensure port is specified
			if err != nil {
				log.Printf("Error relaying query for %s to %s: %v", q.Name, fallbackDNS, err)
				msg.Rcode = dns.RcodeServerFailure
			} else {
				msg = in
			}
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

	// Apply default settings if not specified in config
	configLock.Lock()
	if config.Server.UDP.Port <= 0 {
		config.Server.UDP.Port = 53
	}
	if config.Server.TCP.Port <= 0 {
		config.Server.TCP.Port = 53
	}
	// Default to enabled if not specified
	if !config.Server.UDP.Enabled && !config.Server.TCP.Enabled {
		// If neither is explicitly enabled/disabled, enable both by default
		config.Server.UDP.Enabled = true
		config.Server.TCP.Enabled = true
	}
	configLock.Unlock()

	// Watch for config changes in a goroutine
	go watchConfig()

	// Attach handler function
	dns.HandleFunc(".", handleDNSRequest)

	// Count the number of servers we're starting
	servers := 0

	// Prepare channel for waiting
	errChan := make(chan error)

	// Start UDP server if enabled
	configLock.RLock()
	udpEnabled := config.Server.UDP.Enabled
	udpPort := config.Server.UDP.Port
	udpInterface := config.Server.UDP.Interface
	configLock.RUnlock()

	if udpEnabled {
		servers++
		go func() {
			addr := fmt.Sprintf("%s:%d", udpInterface, udpPort)
			serverUDP := &dns.Server{Addr: addr, Net: "udp"}
			log.Printf("Starting UDP DNS server on %s", addr)
			err := serverUDP.ListenAndServe()
			errChan <- fmt.Errorf("UDP server stopped: %w", err)
		}()
	}

	// Start TCP server if enabled
	configLock.RLock()
	tcpEnabled := config.Server.TCP.Enabled
	tcpPort := config.Server.TCP.Port
	tcpInterface := config.Server.TCP.Interface
	configLock.RUnlock()

	if tcpEnabled {
		servers++
		go func() {
			addr := fmt.Sprintf("%s:%d", tcpInterface, tcpPort)
			serverTCP := &dns.Server{Addr: addr, Net: "tcp"}
			log.Printf("Starting TCP DNS server on %s", addr)
			err := serverTCP.ListenAndServe()
			errChan <- fmt.Errorf("TCP server stopped: %w", err)
		}()
	}

	if servers == 0 {
		log.Fatalf("No DNS servers enabled in configuration")
	}

	// Wait for any server to exit (which is usually an error)
	err := <-errChan
	log.Fatalf("Server error: %v", err)
}
