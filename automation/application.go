package main

import (
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://jandan.net/api/comment/post/26402?order=desc&page=%d"

var _historyPath = path.Join("data", "history.json")
var _userActivity = path.Join("data", "user_activity.csv")

type HistoryRecord struct {
	LastExecution time.Time `json:"last_execution"`
	LastPage      int       `json:"last_page"`
}

func main() {
	_, path, _, _ := runtime.Caller(0)
	historyPath := filepath.Join(filepath.Dir(path), _historyPath)
	userActivity := filepath.Join(filepath.Dir(path), _userActivity)
	fmt.Printf("file path: %s\n", historyPath)
	fmt.Printf("file path: %s\n", userActivity)
	// Attempt to load history to determine a smarter starting page
	hist, _ := loadHistory(historyPath)

	existingIDs := make(map[int]int)
	csvFile, err := os.Open(userActivity)
	if err == nil {
		reader := csv.NewReader(csvFile)
		// Skip header
		_, _ = reader.Read()
		for {
			rec, err := reader.Read()
			if err == io.EOF {
				break
			}
			if len(rec) > 0 {
				id, err := strconv.Atoi(rec[0])
				if err == nil {
					existingIDs[id] = 1
				}
			}
		}
		csvFile.Close()
	}
	fmt.Printf("Loaded %d existing IDs from CSV\n", len(existingIDs))

	cutoff := time.Now().AddDate(0, -1, 0) // one month ago from now
	// Optionally, you could base cutoff on history.LastExecution.
	// Current implementation uses one month ago from now.
	fmt.Printf("Cutoff (one month ago): %s\n", cutoff.Format(time.RFC3339))

	if hist != nil {

		// Ascending: first fetch history page + 1, then get bounds and iterate
		fmt.Println("Resuming from history...")
		fmt.Printf("Last execution: %s, Last page: %d\n", hist.LastExecution.Format(time.RFC3339), hist.LastPage)
		startPage := hist.LastPage
		if startPage < 0 {
			startPage = 0
		}
		initialPage := startPage
		if initialPage < 0 {
			initialPage = 0
		}
		// Fetch initialPage first
		url := fmt.Sprintf(baseURL, initialPage)
		resp, err := fetchPage(url)
		if err != nil {
			fmt.Printf("fetch error for page %d: %v\n", initialPage, err)
			return
		}
		time.Sleep(300 * time.Millisecond)
		// Use bounds from the initial fetch response
		totalPages := 0
		if resp != nil && resp.Data != nil {
			totalPages = resp.Data.TotalPages
		}
		// Prepare CSV writer
		w, f, err := openCSV(userActivity)
		if err != nil {
			fmt.Println("failed to open csv:", err)
			return
		}
		defer f.Close()

		if resp != nil && resp.Data != nil {

			for _, item := range resp.Data.List {
				if _, found := existingIDs[item.ID]; !found {
					fmt.Printf("Appending new item to CSV, ID: %d\n", item.ID)
					_ = appendCSVRecord(w, item)
					w.Flush()
					existingIDs[item.ID] = 1
				}
			}
		}

		// Then iterate from initialPage+1 upwards
		for page := initialPage + 1; page <= totalPages; page++ {
			url := fmt.Sprintf(baseURL, page)
			resp, err := fetchPage(url)
			if err != nil {
				fmt.Printf("fetch error for page %d: %v\n", page, err)
				break
			}
			if resp.Data == nil {
				fmt.Printf("no data for page %d\n", page)
				break
			} else {
				fmt.Printf("Page %d: items=%d (ascending)\n", page, len(resp.Data.List))
				_ = saveHistory(historyPath, HistoryRecord{
					LastExecution: time.Now(),
					LastPage:      page,
				})

				for _, item := range resp.Data.List {
					if _, found := existingIDs[item.ID]; !found {
						// append to CSV
						_ = appendCSVRecord(w, item)
						// flush periodically
						w.Flush()
						existingIDs[item.ID] = 1
					}
				}
			}

			time.Sleep(1000 * time.Millisecond)
		}

		fmt.Println("Stopping iteration due to page reached.")
		// iterate resp.Data.List to check if the id exists in CSV, if not then append
		// Read all IDs from CSV into a map (in memory), then append only new items
		if resp != nil && resp.Data != nil {

			for _, item := range resp.Data.List {
				if _, found := existingIDs[item.ID]; !found {
					fmt.Printf("Appending new item to CSV, ID: %d\n", item.ID)
					_ = appendCSVRecord(w, item)
					w.Flush()
					existingIDs[item.ID] = 1
				}
			}
		}

	} else {
		// Special descending: page=0 first, then total_pages-1, total_pages-2, ... until cutoff

		// Fetch the first page from remote (page=0)
		first, err := fetchPage(fmt.Sprintf(baseURL, 0))
		if err != nil {
			fmt.Println("failed to fetch first page:", err)
			return
		}
		if first.Data == nil {
			fmt.Println("missing data block in first page response")
			return
		}
		// Prepare CSV writer
		w, f, err := openCSV(userActivity)
		if err != nil {
			fmt.Println("failed to open csv:", err)
			return
		}
		defer f.Close()
		// Then iterate descending from total_pages-1 to 0
		firstDescPage := first.Data.CurrentPage
		_ = saveHistory(historyPath, HistoryRecord{
			LastExecution: time.Now(),
			LastPage:      firstDescPage,
		})

		for page := firstDescPage; page >= 0; page-- {
			url := fmt.Sprintf(baseURL, page)
			resp, err := fetchPage(url)
			stop := false
			if err != nil {
				fmt.Printf("fetch error for page %d: %v\n", page, err)
				break
			}
			if resp.Data == nil {
				fmt.Printf("no data for page %d\n", page)
				break
			} else {
				fmt.Printf("Page %d: items=%d (descending)\n", page, len(resp.Data.List))

				for _, item := range resp.Data.List {
					if _, found := existingIDs[item.ID]; !found {
						// append to CSV
						_ = appendCSVRecord(w, item)
						w.Flush()
						existingIDs[item.ID] = 1
					}
					t, perr := parseDate(item.DateGMT)
					if perr != nil {
						continue
					}
					if t.Before(cutoff) || t.Equal(cutoff) {
						fmt.Printf("Reached cutoff at item id=%d date=%s (parsed=%s)\n", item.ID, item.DateGMT, t.Format(time.RFC3339))
						stop = true
						break
					}
				}
				if stop {
					fmt.Println("Stopping iteration due to cutoff reached.")
					break
				}
				time.Sleep(1000 * time.Millisecond)
			}
		}
	}
}

func fetchPage(url string) (*RootResponse, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// Add requested headers
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", "https://jandan.net/pic")
	req.Header.Set("Sec-CH-UA", "\"Chromium\";v=\"142\", \"Google Chrome\";v=\"142\", \"Not_A Brand\";v=\"99\"")
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", "\"macOS\"")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	// Set Host (authority) explicitly
	req.Host = "jandan.net"

	client := &http.Client{}
	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", r.StatusCode)
	}
	var b []byte
	if enc := r.Header.Get("Content-Encoding"); enc == "gzip" {
		gr, gerr := gzip.NewReader(r.Body)
		if gerr != nil {
			return nil, gerr
		}
		defer gr.Close()
		data, rerr := io.ReadAll(gr)
		if rerr != nil {
			return nil, rerr
		}
		b = data
	} else {
		data, rerr := io.ReadAll(r.Body)
		if rerr != nil {
			return nil, rerr
		}
		b = data
	}
	// Optional: quick sanity check
	var tmp RootResponse
	if err := json.Unmarshal(b, &tmp); err != nil {
		return nil, err
	}
	return &tmp, nil
}

// parseDate handles the date_gmt format like "2025-12-11T10:14:36+08:00"
func parseDate(s string) (time.Time, error) {
	// Try parsing as RFC3339 (matches sample)
	return time.Parse(time.RFC3339, s)
}

func loadHistory(path string) (*HistoryRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var h HistoryRecord
	if err := json.Unmarshal(b, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func saveHistory(path string, h HistoryRecord) error {
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	// Ensure directory exists
	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// openCSV opens/creates a CSV file in append mode and writes header if empty
func openCSV(path string) (*csv.Writer, *os.File, error) {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	info, _ := f.Stat()
	w := csv.NewWriter(f)
	if info.Size() == 0 {
		_ = w.Write([]string{"id", "author", "date_gmt", "content"})
		w.Flush()
	}
	return w, f, nil
}

func appendCSVRecord(w *csv.Writer, item Item) error {
	rec := []string{
		fmt.Sprintf("%d", item.ID),
		item.Author,
		item.DateGMT,
		encodeLineBreaks(item.Content),
	}
	if err := w.Write(rec); err != nil {
		return err
	}
	return nil
}

func encodeLineBreaks(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	return html.EscapeString(s)
}
