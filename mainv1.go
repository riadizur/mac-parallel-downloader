package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
)

func getFileSize(url string) (int, error) {
	req, _ := http.NewRequest("HEAD", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	length := resp.Header.Get("Content-Length")
	return strconv.Atoi(length)
}

func downloadRange(url string, start, end, part int, filename string, wg *sync.WaitGroup) {
	defer wg.Done()

	req, _ := http.NewRequest("GET", url, nil)
	rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
	req.Header.Set("Range", rangeHeader)
	req.Header.Set("User-Agent", "curl/8.0 (Go parallel)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error downloading part", part, ":", err)
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(fmt.Sprintf("%s.part%d", filename, part))
	if err != nil {
		fmt.Println("Error creating part file", part, ":", err)
		return
	}
	defer out.Close()

	io.Copy(out, resp.Body)
	fmt.Printf("Finished part %d (%s)\n", part, rangeHeader)
}

func mergeParts(filename string, partCount int) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	for i := 0; i < partCount; i++ {
		partFile := fmt.Sprintf("%s.part%d", filename, i)
		in, err := os.Open(partFile)
		if err != nil {
			return err
		}
		io.Copy(out, in)
		in.Close()
		os.Remove(partFile)
	}
	return nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: go run downloader.go <url> <output_filename> <part_count>")
		return
	}

	url := os.Args[1]
	filename := os.Args[2]
	partCount, _ := strconv.Atoi(os.Args[3])

	size, err := getFileSize(url)
	if err != nil {
		fmt.Println("Failed to get file size:", err)
		return
	}
	fmt.Printf("Total size: %d bytes\n", size)

	chunk := size / partCount
	var wg sync.WaitGroup

	for i := 0; i < partCount; i++ {
		start := i * chunk
		end := start + chunk - 1
		if i == partCount-1 {
			end = size - 1
		}
		wg.Add(1)
		go downloadRange(url, start, end, i, filename, &wg)
	}

	wg.Wait()
	fmt.Println("Merging parts...")
	if err := mergeParts(filename, partCount); err != nil {
		fmt.Println("Error merging:", err)
	} else {
		fmt.Println("Download complete:", filename)
	}
}
