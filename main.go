package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
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

const (
	defaultServerIP = "localhost"
	zertoAPIPort    = 9669
	apiTimeout      = 10 * time.Second
)

func main() {
	serverIP := flag.String("server", defaultServerIP, "ZVM server IP")
	configFile := flag.String("config", "", "Path to the config file")
	flag.Parse()

	if *configFile == "" {
		log.Fatal("Config file path is required")
	}

	config, err := readConfig(*configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: apiTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	sessionToken, err := loginToZerto(client, *serverIP, config.Username, config.Password)
	if err != nil {
		log.Fatalf("Error logging in to Zerto API: %v", err)
	}

	averageRPO, err := queryVPGs(client, *serverIP, sessionToken)
	if err != nil {
		log.Fatalf("Error querying VPGs: %v", err)
	}

	fmt.Println(averageRPO)
}

func readConfig(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func loginToZerto(client *http.Client, serverIP, username, password string) (string, error) {
	loginURL := fmt.Sprintf("https://%s:%d/v1/session/add", serverIP, zertoAPIPort)
	req, _ := http.NewRequest("POST", loginURL, nil)
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to login, status code: %d", resp.StatusCode)
	}

	sessionToken := resp.Header.Get("X-Zerto-Session")
	if sessionToken == "" {
		return "", errors.New("session token not found in headers")
	}

	return sessionToken, nil
}

func queryVPGs(client *http.Client, serverIP, sessionToken string) (int, error) {
	apiURL := fmt.Sprintf("https://%s:%d/v1/vpgs", serverIP, zertoAPIPort)
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("X-Zerto-Session", sessionToken)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var vpgs []VPG
	if err := json.Unmarshal(body, &vpgs); err != nil {
		return 0, fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	if len(vpgs) == 0 {
		return 0, nil
	}

	totalRPO := 0
	for _, vpg := range vpgs {
		totalRPO += vpg.ActualRPO
	}

	return totalRPO / len(vpgs), nil
}
