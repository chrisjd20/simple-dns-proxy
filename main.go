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
	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Port       int      `yaml:"port"`
	Interfaces []string `yaml:"interfaces"`
}

type WeightedIP struct {
	IP     string `yaml:"ip"`
	Weight int    `yaml:"weight"`
}

type RecordConfig struct {
	IP           string       `yaml:"ip,omitempty"`
	AlternateIPs []WeightedIP `yaml:"alternate_ips,omitempty"`
	TTL          *uint32      `yaml:"ttl,omitempty"`
}

type FallbackProxyConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Address  string `yaml:"address"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type Config struct {
	Records          map[string]RecordConfig `yaml:"records"`
	FallbackDNS      string                  `yaml:"fallback_dns"`
	FallbackProtocol string                  `yaml:"fallback_protocol"`
	FallbackProxy    FallbackProxyConfig     `yaml:"fallback_proxy"`
	DefaultTTL       uint32                  `yaml:"default_ttl"`
	Server           struct {
		UDP ServerConfig `yaml:"udp"`
		TCP ServerConfig `yaml:"tcp"`
	} `yaml:"server"`
}

var (
	config             Config
	configLock         sync.RWMutex
	configFile         string                      // Will be set in init()
	defaultConfigPath  = "/app/config/config.yaml" // Default path inside the container
	recordCounters     map[string]int
	recordCountersLock sync.Mutex
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

	// Reset counters on successful reload
	recordCountersLock.Lock()
	recordCounters = make(map[string]int)
	recordCountersLock.Unlock()

	configLock.Lock()
	defer configLock.Unlock()
	config = newConfig

	// Apply default values
	if config.DefaultTTL <= 0 {
		config.DefaultTTL = 3600 // Default to 1 hour
	}

	// Apply default values for server if not specified
	if config.Server.UDP.Port <= 0 {
		config.Server.UDP.Port = 53
	}
	if len(config.Server.UDP.Interfaces) == 0 {
		config.Server.UDP.Interfaces = []string{"0.0.0.0"}
	}
	if config.Server.TCP.Port <= 0 {
		config.Server.TCP.Port = 53
	}
	if len(config.Server.TCP.Interfaces) == 0 {
		config.Server.TCP.Interfaces = []string{"0.0.0.0"}
	}

	log.Println("Configuration loaded/reloaded")
	log.Printf("Records: %v", config.Records)
	log.Printf("Fallback DNS: %s", config.FallbackDNS)
	log.Printf("Fallback Protocol: %s", config.FallbackProtocol)
	log.Printf("Fallback Proxy: enabled=%v, address=%s", config.FallbackProxy.Enabled, config.FallbackProxy.Address)
	log.Printf("Default TTL: %d", config.DefaultTTL)

	// Log server configuration
	log.Printf("UDP Server: enabled=%v, port=%d, interfaces=%q",
		config.Server.UDP.Enabled, config.Server.UDP.Port, config.Server.UDP.Interfaces)
	log.Printf("TCP Server: enabled=%v, port=%d, interfaces=%q",
		config.Server.TCP.Enabled, config.Server.TCP.Port, config.Server.TCP.Interfaces)

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
		fallbackProto := config.FallbackProtocol
		fallbackProxy := config.FallbackProxy
		configLock.RUnlock()

		// For A records, check if we have a match in our config first
		if q.Qtype == dns.TypeA {
			configLock.RLock()
			record, exists := config.Records[strings.ToLower(strings.TrimSuffix(q.Name, "."))]
			defaultTTL := config.DefaultTTL
			configLock.RUnlock()

			if exists {
				var ip string
				// Weighted round-robin logic
				if len(record.AlternateIPs) > 0 {
					totalWeight := 0
					for _, wip := range record.AlternateIPs {
						totalWeight += wip.Weight
					}

					if totalWeight > 0 {
						recordCountersLock.Lock()
						counter := recordCounters[q.Name]
						recordCounters[q.Name]++
						recordCountersLock.Unlock()

						currentCountInCycle := counter % totalWeight
						runningWeight := 0
						for _, wip := range record.AlternateIPs {
							runningWeight += wip.Weight
							if currentCountInCycle < runningWeight {
								ip = wip.IP
								break
							}
						}
					}
				} else {
					ip = record.IP
				}

				if ip == "" {
					log.Printf("No IP found for %s, either misconfigured or single IP not set.", q.Name)
					continue
				}

				var ttl uint32
				if record.TTL != nil {
					ttl = *record.TTL
				} else {
					ttl = defaultTTL
				}

				log.Printf("Found A record for %s -> %s with TTL %d", q.Name, ip, ttl)
				rr, err := dns.NewRR(fmt.Sprintf("%s %d IN A %s", q.Name, ttl, ip))
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
			netProto := fallbackProto
			if netProto == "" {
				netProto = w.RemoteAddr().Network()
			}

			var in *dns.Msg
			var err error

			if fallbackProxy.Enabled && fallbackProxy.Address != "" {
				if netProto == "udp" {
					log.Printf("Warning: SOCKS5 proxy is enabled but fallback protocol is UDP. Proxying UDP is not supported. Bypassing proxy for %s", q.Name)
					c := new(dns.Client)
					c.Net = netProto
					in, _, err = c.Exchange(r, fallbackDNS+":53")
				} else {
					log.Printf("Using SOCKS5 proxy %s for fallback query for %s", fallbackProxy.Address, q.Name)
					var auth *proxy.Auth
					if fallbackProxy.Username != "" {
						auth = &proxy.Auth{
							User:     fallbackProxy.Username,
							Password: fallbackProxy.Password,
						}
					}
					dialer, errDial := proxy.SOCKS5("tcp", fallbackProxy.Address, auth, proxy.Direct)
					if errDial != nil {
						err = fmt.Errorf("failed to create SOCKS5 dialer: %w", errDial)
					} else {
						conn, errDialConn := dialer.Dial(netProto, fallbackDNS+":53")
						if errDialConn != nil {
							err = fmt.Errorf("failed to dial via SOCKS5 proxy: %w", errDialConn)
						} else {
							co := &dns.Conn{Conn: conn}
							errWrite := co.WriteMsg(r)
							if errWrite != nil {
								err = fmt.Errorf("failed to write message via proxy: %w", errWrite)
							} else {
								in, err = co.ReadMsg()
							}
							co.Close()
						}
					}
				}
			} else {
				// Relay to fallback DNS without proxy
				c := new(dns.Client)
				c.Net = netProto
				in, _, err = c.Exchange(r, fallbackDNS+":53")
			}

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
	recordCounters = make(map[string]int)

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
	udpInterfaces := config.Server.UDP.Interfaces
	configLock.RUnlock()

	if udpEnabled {
		for _, iface := range udpInterfaces {
			servers++
			go func(iface string) {
				addr := fmt.Sprintf("%s:%d", iface, udpPort)
				serverUDP := &dns.Server{Addr: addr, Net: "udp"}
				log.Printf("Starting UDP DNS server on %s", addr)
				err := serverUDP.ListenAndServe()
				errChan <- fmt.Errorf("UDP server on %s stopped: %w", addr, err)
			}(iface)
		}
	}

	// Start TCP server if enabled
	configLock.RLock()
	tcpEnabled := config.Server.TCP.Enabled
	tcpPort := config.Server.TCP.Port
	tcpInterfaces := config.Server.TCP.Interfaces
	configLock.RUnlock()

	if tcpEnabled {
		for _, iface := range tcpInterfaces {
			servers++
			go func(iface string) {
				addr := fmt.Sprintf("%s:%d", iface, tcpPort)
				serverTCP := &dns.Server{Addr: addr, Net: "tcp"}
				log.Printf("Starting TCP DNS server on %s", addr)
				err := serverTCP.ListenAndServe()
				errChan <- fmt.Errorf("TCP server on %s stopped: %w", addr, err)
			}(iface)
		}
	}

	if servers == 0 {
		log.Fatalf("No DNS servers enabled in configuration")
	}

	// Wait for any server to exit (which is usually an error)
	err := <-errChan
	log.Fatalf("Server error: %v", err)
}
