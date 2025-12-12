package main

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genai"
)

func main() {
	csvPath := filepath.Join("..", "crawler", "data", "user_activity.csv")
	posts, err := ReadPostsFromCSV(csvPath)
	if err != nil {
		log.Fatalf("Failed to read posts: %v", err)
	}
	// 1. filter out posts older than 3 days ago
	// 2. group by user
	// 3. find the top 1 post with most VoteNegative for each user

	// Parse DateGMT as time.Time, filter, group, and select top post
	importTime := func(dateStr string) (time.Time, error) {
		// Try common formats, adjust as needed
		layouts := []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02"}
		var t time.Time
		var err error
		for _, layout := range layouts {
			t, err = time.Parse(layout, dateStr)
			if err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("unrecognized date format: %s", dateStr)
	}

	now := time.Now().UTC()
	threeDaysAgo := now.AddDate(0, 0, -3)

	filtered := make([]Post, 0, len(posts))
	for _, post := range posts {
		t, err := importTime(post.DateGMT)
		if err != nil {
			continue // skip if date can't be parsed
		}
		if t.After(threeDaysAgo) {
			filtered = append(filtered, post)
		}
	}

	// Group by user
	userPosts := make(map[string][]Post)
	for _, post := range filtered {
		userPosts[post.Author] = append(userPosts[post.Author], post)
	}

	// Find top 1 post with most VoteNegative for each user
	type UserTopPost struct {
		User string
		Post Post
	}
	var topPosts []UserTopPost
	for user, posts := range userPosts {
		if len(posts) == 0 {
			continue
		}
		top := posts[0]
		for _, p := range posts[1:] {
			if p.VoteNegative > top.VoteNegative {
				top = p
			}
		}
		topPosts = append(topPosts, UserTopPost{User: user, Post: top})
	}

	// Print results
	fmt.Printf("Top 1 post with most VoteNegative for each user (last 3 days): %d\n", len(topPosts))
	for _, utp := range topPosts {
		url, mimeType := ExtractImgSrcs(utp.Post.Content)
		if len(url) < 1 {
			continue
		}
		imageBytes, err := downloadImage(url)
		if err != nil {
			log.Printf("%v", err)
			continue
		}

		shouldBlock, err := analyzeContentWithGenAI(imageBytes, mimeType)
		if err != nil {
			log.Printf("Failed to analyze image content: %v", err)
		}
		if shouldBlock {
			fmt.Printf("User: %s, Post ID: %d, Image URL: %s is flagged by GenAI analysis.\n", utp.User, utp.Post.ID, url)
		} else {
			fmt.Printf("User: %s, Post ID: %d, Image URL: %s is clean.\n", utp.User, utp.Post.ID, url)
		}
		break
	}
}

func analyzeContentWithGenAI(content []byte, mimeType string) (bool, error) {
	ctx := context.Background()
	// The client gets the API key from the environment variable `GEMINI_API_KEY`.
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	parts := []*genai.Part{
		genai.NewPartFromBytes(content, mimeType),
		genai.NewPartFromText(`Please respond with only 'yes' or 'no.' Is the image a screenshot from a social media platform?Does the text in the image incite gender conflicts?Does the image contain content that may be unsettling (e.g., snakes, spiders)?`),
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	result, err := client.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash",
		contents,
		nil,
	)
	if err != nil {
		return false, err
	}

	if strings.Contains(strings.ToLower(result.Text()), "yes") {
		return true, nil
	} else {
		return false, nil
	}
}

func downloadImage(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %v", url, err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	client := &http.Client{}
	imageResp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image from %s: %v", url, err)
	}
	defer imageResp.Body.Close()

	var reader io.Reader = imageResp.Body
	switch imageResp.Header.Get("Content-Encoding") {
	case "gzip":
		gzReader, err := gzip.NewReader(imageResp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader for %s: %v", url, err)
		}
		defer gzReader.Close()
		reader = gzReader
		// Add more encodings if needed (e.g., deflate, br)
	}

	imageBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read image body from %s: %v", url, err)
	}
	return imageBytes, nil
}

// ReadPostsFromCSV reads posts from a CSV file and returns a slice of Post structs.
func ReadPostsFromCSV(path string) ([]Post, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // allow variable number of fields

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var posts []Post
	for i, rec := range records {
		if i == 0 {
			// skip header
			continue
		}
		if len(rec) < 6 {
			continue // skip incomplete rows
		}
		id, _ := strconv.Atoi(rec[0])
		voteNeg, _ := strconv.Atoi(rec[3])
		votePos, _ := strconv.Atoi(rec[4])
		content := html.UnescapeString(rec[5])
		post := Post{
			ID:           id,
			Author:       rec[1],
			DateGMT:      rec[2],
			Content:      content,
			VoteNegative: voteNeg,
			VotePositive: votePos,
		}
		posts = append(posts, post)
	}
	return posts, nil
}

type Post struct {
	ID           int
	Author       string
	DateGMT      string
	Content      string
	VoteNegative int
	VotePositive int
}

// ExtractImgSrcs extracts all src values from img tags in the given HTML content.
const imgSrcRegex = `<img\s+[^>]*src=["']([^"']+)["']`

var imgSrcRe = regexp.MustCompile(imgSrcRegex)

func ExtractImgSrcs(content string) (string, string) {
	matches := imgSrcRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		mimeType := ""
		isJpg := strings.HasSuffix(m[1], "jpg") || strings.HasSuffix(m[1], "jpeg")
		isPng := strings.HasSuffix(m[1], "png")
		if len(m) > 1 && (isJpg || isPng) {
			if isJpg {
				mimeType = "image/jpeg"
			} else if isPng {
				mimeType = "image/png"
			}
			return m[1], mimeType
		}
	}
	return "", ""
}
