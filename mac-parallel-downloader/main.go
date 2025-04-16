package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

func getFileSize(url string) (int, error) {
	resp, err := http.Head(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	sizeStr := resp.Header.Get("Content-Length")
	if sizeStr == "" {
		return 0, fmt.Errorf("no Content-Length header")
	}
	return strconv.Atoi(sizeStr)
}

func downloadPart(url string, start, end, index int, filename string, wg *sync.WaitGroup, p *mpb.Progress) {
	defer wg.Done()

	size := end - start + 1
	bar := p.New(int64(size),
		mpb.BarStyle().Rbound("|").Lbound("|").Filler("█").Tip("▉").Padding(" "),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Part %02d-", index), decor.WC{W: 8, C: decor.DSyncWidth}),
			decor.CountersKibiByte("% .2f / % .2f"),
		),
		mpb.AppendDecorators(decor.Percentage(decor.WC{W: 5})),
	)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", "Go-Downloader")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error downloading part", index, ":", err)
		return
	}
	defer resp.Body.Close()

	partFile := fmt.Sprintf("%s.part%d", filename, index)
	out, err := os.Create(partFile)
	if err != nil {
		fmt.Println("Error creating part file:", err)
		return
	}
	defer out.Close()

	proxyReader := bar.ProxyReader(resp.Body)
	defer proxyReader.Close()

	io.Copy(out, proxyReader)
}

func mergeParts(filename string, count int) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	for i := 0; i < count; i++ {
		partName := fmt.Sprintf("%s.part%d", filename, i)
		in, err := os.Open(partName)
		if err != nil {
			return err
		}

		_, err = io.Copy(out, in)
		in.Close()
		os.Remove(partName)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: downloader <url> <output_filename> <part_count>")
		return
	}

	url := os.Args[1]
	filename := os.Args[2]
	partCount, err := strconv.Atoi(os.Args[3])
	if err != nil || partCount <= 0 {
		fmt.Println("Invalid part count. Must be a positive integer.")
		return
	}

	fmt.Println("Starting download...")
	size, err := getFileSize(url)
	if err != nil {
		fmt.Println("Error getting file size:", err)
		return
	}
	fmt.Printf("File size: %.2f MiB\n\n", float64(size)/1024/1024)

	startTime := time.Now()

	chunkSize := size / partCount
	var wg sync.WaitGroup
	p := mpb.New(mpb.WithWaitGroup(&wg), mpb.WithRefreshRate(150*time.Millisecond))

	for i := 0; i < partCount; i++ {
		start := i * chunkSize
		end := start + chunkSize - 1
		if i == partCount-1 {
			end = size - 1
		}

		wg.Add(1)
		go downloadPart(url, start, end, i, filename, &wg, p)
	}

	wg.Wait()
	p.Wait()

	fmt.Println("\nMerging parts...")
	if err := mergeParts(filename, partCount); err != nil {
		fmt.Println("Merge error:", err)
	} else {
		elapsed := time.Since(startTime)
		seconds := elapsed.Seconds()
		speedMiB := (float64(size) / 1024 / 1024) / seconds
		fmt.Printf("✅ Download complete: %s\n", filename)
		fmt.Printf("⏱  Total time   : %.2f seconds\n", seconds)
		fmt.Printf("🚀 Avg speed    : %.2f MiB/s\n", speedMiB)
	}
}
