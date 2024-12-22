package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"
)

// VPG struct represents the VPG details returned by the Zerto API
type VPG struct {
	ActualRPO int `json:"ActualRPO"`
}

// Config struct holds the ZVM login credentials
type Config struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	// Accept command-line arguments for server IP and config file path
	serverIP := flag.String("server", "localhost", "ZVM server IP")
	configFile := flag.String("config", "", "Path to the config file")
	flag.Parse()

	if *configFile == "" {
		log.Fatal("Config file path is required")
	}

	// Read config file
	config, err := readConfig(*configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// Create HTTP client with custom transport to skip TLS verification
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Skip certificate validation
		},
	}

	// Login to Zerto API and get session token
	sessionToken, err := loginToZerto(client, *serverIP, config.Username, config.Password)
	if err != nil {
		log.Fatalf("Error logging in to Zerto API: %v", err)
	}

	// Query VPGs from Zerto API using session token
	err = queryVPGs(client, *serverIP, sessionToken)
	if err != nil {
		log.Fatalf("Error querying VPGs: %v", err)
	}
}

// readConfig reads the config file and returns the username and password
func readConfig(configFile string) (*Config, error) {
	bytes, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(bytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// loginToZerto handles logging into the Zerto API and returning a session token
func loginToZerto(client *http.Client, serverIP, username, password string) (string, error) {
	loginURL := fmt.Sprintf("https://%s:9669/v1/session/add", serverIP)

	// Explicit JSON payload for authentication
	payload := map[string]string{"username": username, "password": password}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error creating JSON payload: %v", err)
	}

	req, err := http.NewRequest("POST", loginURL, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed, status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	// Extract session token from the response
	sessionToken := resp.Header.Get("X-Zerto-Session")
	if sessionToken == "" {
		return "", fmt.Errorf("session token not found in headers")
	}

	return sessionToken, nil
}

// queryVPGs queries the VPGs and returns the average Actual RPO as an integer
func queryVPGs(client *http.Client, serverIP, sessionToken string) error {
	apiURL := fmt.Sprintf("https://%s/v1/vpgs", serverIP)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("X-Zerto-Session", sessionToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error querying VPGs, status code: %d, response: %s", resp.StatusCode, string(body))
	}

	// Unmarshal the response into a slice of VPGs
	var vpgs []VPG
	if err := json.Unmarshal(body, &vpgs); err != nil {
		return fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	// Calculate the average RPO
	totalRPO := 0
	for _, vpg := range vpgs {
		totalRPO += vpg.ActualRPO
	}

	if len(vpgs) > 0 {
		averageRPO := totalRPO / len(vpgs)
		fmt.Printf("%d\n", averageRPO)
	} else {
		fmt.Println("0")
	}

	return nil
}
