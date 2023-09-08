package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Define a function to print a progress bar.
func printProgressBar(iteration, total int, length int) {
	percent := float64(iteration) / float64(total)
	filledLength := int(float64(length) * percent)
	bar := strings.Repeat("=", filledLength) + strings.Repeat("-", length-filledLength)
	fmt.Printf("\r[%s] %.2f%% Complete", bar, percent*100)
}

func main() {
	// Command-line flags for file, rate, threads, timeout, and output file
	fileFlag := flag.String("file", "cloud.txt", "Input file containing URLs (domains or IPs)")
	rateFlag := flag.Int("rate", 200, "Scan rate (hosts per second)")
	threadsFlag := flag.Int("threads", 100, "Number of concurrent threads")
	timeoutFlag := flag.Duration("timeout", 3*time.Second, "Timeout for HTTP requests")
	outputFlag := flag.String("o", "", "Output file for working hosts")
	varnishFlag := flag.Bool("varnish", false, "Download varnish IP ranges")
	flag.Parse()

	if *varnishFlag {
		fmt.Println("Downloading varnish IP ranges...")
		resp, err := http.Get("https://raw.githubusercontent.com/elon30/varnish/main/varnish.txt")
		if err != nil {
			fmt.Println("Error downloading varnish.txt:", err)
			return
		}
		defer resp.Body.Close()

		// Create a progress bar for the download
		progressBarLength := 40
		buffer := make([]byte, 1024)
		var totalSize int64

		// Create a new file to save the downloaded content
		varnishFile, err := os.Create("varnish.txt")
		if err != nil {
			fmt.Println("Error creating varnish.txt:", err)
			return
		}
		defer varnishFile.Close()

		for {
			n, err := resp.Body.Read(buffer)
			if err == io.EOF {
				break
			} else if err != nil {
				fmt.Println("Error reading from varnish.txt:", err)
				return
			}

			// Write the downloaded data to the file
			_, err = varnishFile.Write(buffer[:n])
			if err != nil {
				fmt.Println("Error writing to varnish.txt:", err)
				return
			}

			totalSize += int64(n)
			printProgressBar(int(totalSize), int(totalSize), progressBarLength)
		}

		fmt.Println("\nDownload completed. Saved as varnish.txt")
	}

	// Open and read the input file
	file, err := os.Open(*fileFlag)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Create a WaitGroup to wait for Goroutines to finish
	var wg sync.WaitGroup

	// Create a channel to manage the worker pool
	jobQueue := make(chan string)
	defer close(jobQueue)

	// Start worker Goroutines
	for i := 0; i < *threadsFlag; i++ {
		go worker(jobQueue, &wg, *timeoutFlag, *outputFlag)
	}

	// Calculate the interval between scans based on the rate
	interval := time.Second / time.Duration(*rateFlag)

	// Count the total number of lines in the file
	totalLines := 0
	for scanner.Scan() {
		totalLines++
	}

	// Reset the scanner to the beginning of the file
	file.Seek(0, 0)
	scanner = bufio.NewScanner(file)

	// Initialize progress bar parameters
	progressBarLength := 40
	currentLine := 0

	// Create an output file if specified
	var outputFile *os.File
	if *outputFlag != "" {
		outputFile, err = os.Create(*outputFlag)
		if err != nil {
			fmt.Println("Error creating output file:", err)
			return
		}
		defer outputFile.Close()
	}

	// Scan and process each line
	for scanner.Scan() {
		url := scanner.Text()

		// Check if the URL has a protocol scheme, if not, add "http://"
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "http://" + url
		}

		// Enqueue the URL for scanning
		jobQueue <- url

		// Increment the current line count
		currentLine++

		// Print the progress bar
		printProgressBar(currentLine, totalLines, progressBarLength)

		// Sleep for the calculated interval to control the rate
		time.Sleep(interval)
	}

	// Wait for all Goroutines to finish
	wg.Wait()

	// Print a newline to separate the progress bar
	fmt.Println()
}

func worker(jobQueue chan string, wg *sync.WaitGroup, timeout time.Duration, outputFileName string) {
	for url := range jobQueue {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			resp, err := httpClientWithTimeout(url, timeout)
			if err != nil {
				// Ignore errors and continue to the next URL
				return
			}

			// Extract and print the IP/domain and response status
			ipOrDomain := strings.TrimPrefix(url, "http://")
			ipOrDomain = strings.TrimPrefix(ipOrDomain, "https://")
			status := fmt.Sprintf("%s %s\n", ipOrDomain, resp.Status)

			// Print the result to the console
			fmt.Print(status)

			// Write to the output file if specified
			if outputFileName != "" {
				outputFile, err := os.OpenFile(outputFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					fmt.Println("Error opening output file:", err)
					return
				}
				defer outputFile.Close()

				_, err = outputFile.WriteString(status)
				if err != nil {
					fmt.Println("Error writing to output file:", err)
				}
			}
		}(url)
	}
}

func httpClientWithTimeout(url string, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return resp, nil
}
