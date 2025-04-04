package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// Structures to organize the data
type PhoneData struct {
	IP       string
	HostName string
	TFTP     string
	CUCM     string
}

type DownloadTarget struct {
	URL      string
	SavePath string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: program <ip_file>")
		os.Exit(1)
	}
	fileName := os.Args[1]

	// Ensure output directory exists
	err := os.MkdirAll("output", 0755)
	if err != nil {
		fmt.Println("Error creating output directory:", err)
		os.Exit(1)
	}

	fmt.Println(`
   _____ _____ _____ _____ _           
  |     |  |  |     |     | |_ ___ ___ 
  |   --|  |  |   --| | | | . | -_|  _|
  |_____|_____|_____|_|_|_|___|___|_|
    CUCMber - by cola`)

	// Create channels and wait groups
	ipChan := make(chan string, 100)
	resultChan := make(chan PhoneData, 100)

	var scrapeWg sync.WaitGroup
	var processWg sync.WaitGroup

	// Data stores with thread-safe access
	var serversMutex sync.Mutex
	tftpServers := make(map[string]bool)
	cucmServers := make(map[string]bool)
	hostnames := make(map[string]bool)

	// Maps to track which hostnames belong to which servers
	serverHostnames := make(map[string][]string)

	// Define number of workers
	const numScrapers = 20

	// Start the result processor
	processWg.Add(1)
	go func() {
		defer processWg.Done()

		for result := range resultChan {
			serversMutex.Lock()

			// Store servers and hostnames with deduplication
			if result.TFTP != "" && result.TFTP != "::" {
				tftpServers[result.TFTP] = true

				// Associate hostname with this server
				if result.HostName != "" {
					// Check if we need to initialize the slice
					if serverHostnames[result.TFTP] == nil {
						serverHostnames[result.TFTP] = []string{}
					}

					// Check if hostname already exists for this server
					exists := false
					for _, h := range serverHostnames[result.TFTP] {
						if h == result.HostName {
							exists = true
							break
						}
					}

					if !exists {
						serverHostnames[result.TFTP] = append(serverHostnames[result.TFTP], result.HostName)
					}
				}
			}

			if result.CUCM != "" && result.CUCM != "::" {
				cucmServers[result.CUCM] = true

				// Associate hostname with this server
				if result.HostName != "" {
					// Check if we need to initialize the slice
					if serverHostnames[result.CUCM] == nil {
						serverHostnames[result.CUCM] = []string{}
					}

					// Check if hostname already exists for this server
					exists := false
					for _, h := range serverHostnames[result.CUCM] {
						if h == result.HostName {
							exists = true
							break
						}
					}

					if !exists {
						serverHostnames[result.CUCM] = append(serverHostnames[result.CUCM], result.HostName)
					}
				}
			}

			if result.HostName != "" {
				hostnames[result.HostName] = true
			}

			serversMutex.Unlock()

			fmt.Printf("Processed data from %s - Hostname: %s, TFTP: %s, CUCM: %s\n",
				result.IP, result.HostName, result.TFTP, result.CUCM)
		}
	}()

	// Start the scrapers
	for i := 0; i < numScrapers; i++ {
		scrapeWg.Add(1)
		go scrapeWorker(ipChan, resultChan, &scrapeWg)
	}

	// Read IPs from file and send to channel
	f, err := os.Open(fileName)
	if err != nil {
		fmt.Println("Error opening file:", err)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		if ip != "" {
			ipChan <- ip
		}
	}
	f.Close()

	if err := scanner.Err(); err != nil {
		fmt.Println("Scanner error:", err)
	}

	// Close IP channel to signal no more IPs
	close(ipChan)

	// Wait for all scrapers to finish
	scrapeWg.Wait()

	// Close the result channel
	close(resultChan)

	// Wait for processor to finish
	processWg.Wait()

	// Debug output
	fmt.Printf("\n--- Found %d TFTP servers, %d CUCM servers, and %d unique hostnames ---\n\n",
		len(tftpServers), len(cucmServers), len(hostnames))

	// Build download targets
	downloadTargets := buildDownloadTargets(tftpServers, cucmServers, serverHostnames)

	// Download all targets
	downloadFiles(downloadTargets)

	fmt.Println("\nAll operations completed successfully.")
}

// Worker to scrape phone web pages
func scrapeWorker(ipChan <-chan string, resultChan chan<- PhoneData, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for ip := range ipChan {
		result := PhoneData{IP: ip}

		// Try to get data from serviceability page
		servicePage := fmt.Sprintf("http://%s/CGI/Java/Serviceability?adapter=device.statistics.configuration", ip)
		if resp, err := client.Get(servicePage); err == nil {
			hostname, tftp, cucm, err := parseHTML(resp.Body)
			resp.Body.Close()

			if err == nil {
				result.HostName = hostname
				result.TFTP = tftp
				result.CUCM = cucm

				fmt.Printf("%s: service page found hostname=%s, tftp=%s, cucm=%s\n",
					ip, hostname, tftp, cucm)
			}
		}

		// Try to get data from network configuration page
		networkPage := fmt.Sprintf("http://%s/NetworkConfiguration", ip)
		if resp, err := client.Get(networkPage); err == nil {
			hostname, tftp, cucm, err := parseHTML(resp.Body)
			resp.Body.Close()

			if err == nil {
				// If we didn't get these from the first page, use these values
				if result.HostName == "" {
					result.HostName = hostname
				}
				if result.TFTP == "" {
					result.TFTP = tftp
				}
				if result.CUCM == "" {
					result.CUCM = cucm
				}

				fmt.Printf("%s: network page found hostname=%s, tftp=%s, cucm=%s\n",
					ip, hostname, tftp, cucm)
			}
		}

		// Only send results if we found something useful
		if result.HostName != "" || result.TFTP != "" || result.CUCM != "" {
			resultChan <- result
		} else {
			fmt.Printf("%s: No useful data found\n", ip)
		}
	}
}

// Build a list of all files to download
func buildDownloadTargets(tftpServers, cucmServers map[string]bool, serverHostnames map[string][]string) []DownloadTarget {
	var targets []DownloadTarget
	var targetsMutex sync.Mutex
	var wg sync.WaitGroup

	// Process all TFTP and CUCM servers
	servers := make([]string, 0)
	for server := range tftpServers {
		servers = append(servers, server)
	}
	for server := range cucmServers {
		// Check if this CUCM server is already in our list (in case it's also a TFTP server)
		found := false
		for _, s := range servers {
			if s == server {
				found = true
				break
			}
		}
		if !found {
			servers = append(servers, server)
		}
	}

	// Process each server
	for _, server := range servers {
		wg.Add(1)
		go func(server string) {
			defer wg.Done()

			// Standard files for every server
			standardTargets := []DownloadTarget{
				{
					URL:      fmt.Sprintf("http://%s:6970/SPDefault.cnf.xml", server),
					SavePath: fmt.Sprintf("output/%s.SPDefault.cnf.xml", server),
				},
				{
					URL:      fmt.Sprintf("http://%s:6970/SPDefault.cnf.xml.sgn", server),
					SavePath: fmt.Sprintf("output/%s.SPDefault.cnf.xml.sgn", server),
				},
				{
					URL:      fmt.Sprintf("http://%s:6970/ConfigFileCacheList.txt", server),
					SavePath: fmt.Sprintf("output/%s.ConfigFileCacheList.txt", server),
				},
				{
					URL:      fmt.Sprintf("http://%s:6970/ConfigFileCacheList.txt.sgn", server),
					SavePath: fmt.Sprintf("output/%s.ConfigFileCacheList.txt.sgn", server),
				},
			}

			targetsMutex.Lock()
			targets = append(targets, standardTargets...)
			targetsMutex.Unlock()

			// Try to get the cache list
			cacheListURL := fmt.Sprintf("http://%s:6970/ConfigFileCacheList.txt", server)
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(cacheListURL)

			if err == nil && resp.StatusCode == http.StatusOK {
				scanner := bufio.NewScanner(resp.Body)
				var cacheTargets []DownloadTarget

				for scanner.Scan() {
					line := scanner.Text()
					parts := strings.Fields(line)
					if len(parts) > 0 && strings.Contains(parts[0], ".") {
						filename := parts[0]

						cacheTargets = append(cacheTargets, DownloadTarget{
							URL:      fmt.Sprintf("http://%s:6970/%s", server, filename),
							SavePath: fmt.Sprintf("output/%s.%s", server, filename),
						})

						// Also try to download the .sgn version if not already a .sgn file
						if !strings.HasSuffix(filename, ".sgn") {
							cacheTargets = append(cacheTargets, DownloadTarget{
								URL:      fmt.Sprintf("http://%s:6970/%s.sgn", server, filename),
								SavePath: fmt.Sprintf("output/%s.%s.sgn", server, filename),
							})
						}
					}
				}

				resp.Body.Close()

				targetsMutex.Lock()
				targets = append(targets, cacheTargets...)
				targetsMutex.Unlock()

				fmt.Printf("Added %d files from cache list for server %s\n", len(cacheTargets), server)
			}

			// Add hostname-specific files
			if hostnames, ok := serverHostnames[server]; ok {
				var hostnameTargets []DownloadTarget

				for _, hostname := range hostnames {
					hostnameTargets = append(hostnameTargets,
						DownloadTarget{
							URL:      fmt.Sprintf("http://%s:6970/%s.cnf.xml", server, hostname),
							SavePath: fmt.Sprintf("output/%s.%s.cnf.xml", server, hostname),
						},
						DownloadTarget{
							URL:      fmt.Sprintf("http://%s:6970/%s.cnf.xml.sgn", server, hostname),
							SavePath: fmt.Sprintf("output/%s.%s.cnf.xml.sgn", server, hostname),
						},
					)
				}

				targetsMutex.Lock()
				targets = append(targets, hostnameTargets...)
				targetsMutex.Unlock()

				fmt.Printf("Added %d hostname-specific files for server %s\n", len(hostnameTargets), server)
			}
		}(server)
	}

	// Wait for all servers to be processed
	wg.Wait()

	fmt.Printf("Built a total of %d download targets\n", len(targets))
	return targets
}

// Download all files in parallel
func downloadFiles(targets []DownloadTarget) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 20) // Limit to 20 concurrent downloads

	for _, target := range targets {
		wg.Add(1)
		go func(target DownloadTarget) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create a client with timeout
			client := &http.Client{
				Timeout: 15 * time.Second,
			}

			// Attempt download
			resp, err := client.Get(target.URL)
			if err != nil {
				fmt.Printf("Error downloading %s: %v\n", target.URL, err)
				return
			}
			defer resp.Body.Close()

			// Check for successful response
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Failed to download %s: HTTP %d\n", target.URL, resp.StatusCode)
				return
			}

			// Create the output file
			outFile, err := os.Create(target.SavePath)
			if err != nil {
				fmt.Printf("Error creating file %s: %v\n", target.SavePath, err)
				return
			}

			// Copy the data
			n, err := io.Copy(outFile, resp.Body)
			outFile.Close()

			if err != nil {
				fmt.Printf("Error saving to %s: %v\n", target.SavePath, err)
				os.Remove(target.SavePath) // Remove file on error
				return
			}

			// Don't keep empty/tiny files
			if n < 10 {
				fmt.Printf("Removing empty file %s (%d bytes)\n", target.SavePath, n)
				os.Remove(target.SavePath)
				return
			}

			fmt.Printf("Saved %s (%d bytes)\n", target.SavePath, n)
		}(target)
	}

	// Wait for all downloads to complete
	wg.Wait()
}

// Extract hostname and server information from HTML
func parseHTML(r io.Reader) (hostName, tftp string, ucm string, err error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", "", "", err
	}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			label, value := extractRow(n)
			lowerLabel := strings.ToLower(label)
			switch {
			case strings.Contains(lowerLabel, "host name") && hostName == "":
				hostName = value
			case strings.Contains(lowerLabel, "tftp server 1") && tftp == "":
				tftp = value
			case strings.Contains(lowerLabel, "unified cm 1") && ucm == "":
				ucmarray := strings.Split(value, " ")
				ucm = ucmarray[0]
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	if hostName == "" && tftp == "" && ucm == "" {
		return hostName, tftp, ucm, fmt.Errorf("could not find required fields")
	}
	return hostName, tftp, ucm, nil
}

// Support rows with either two cells (label, value) or three (label, spacer, value)
func extractRow(n *html.Node) (label, value string) {
	var tds []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "td" {
			text := extractText(c)
			tds = append(tds, strings.TrimSpace(text))
		}
	}
	if len(tds) == 2 {
		label = tds[0]
		value = tds[1]
	} else if len(tds) >= 3 {
		label = tds[0]
		value = tds[2]
	}
	return label, value
}

func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var s string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s += extractText(c)
	}
	return s
}
