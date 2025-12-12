package main

import (
	"bufio"
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
	blockedUsers, err := readBlockedUsers(filepath.Join("..", "blocked_users.txt"))
	if err != nil {
		log.Printf("Failed to read blocked users: %v\n", err)
	}
	csvPath := filepath.Join("..", "crawler", "data", "user_activity.csv")
	posts, err := ReadPostsFromCSV(csvPath)
	if err != nil {
		log.Fatalf("Failed to read posts: %v", err)
	}

	filtered := filterRecentPosts(posts, 3, blockedUsers)
	userPosts := groupPostsByUser(filtered)
	topPosts := getTopPostsByVoteNegative(userPosts)

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
			break
		}
		if shouldBlock {
			// add the user to blocked list and persist
			if _, exists := blockedUsers[utp.User]; !exists {
				blockedUsers[utp.User] = 1
				err := appendBlockedUser(filepath.Join("..", "blocked_users.txt"), utp.User)
				if err != nil {
					log.Printf("Failed to append blocked user: %v", err)
				} else {
					fmt.Printf("User: %s has been added to the blocked list.\n", utp.User)
				}
			}
			fmt.Printf("User: %s, Post ID: %d, Image URL: %s is flagged by GenAI analysis.\n", utp.User, utp.Post.ID, url)
		} else {
			fmt.Printf("User: %s, Post ID: %d, Image URL: %s is clean.\n", utp.User, utp.Post.ID, url)
		}
	}
}

// appendBlockedUser appends a username to the blocked users file.
func appendBlockedUser(path, user string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(user + "\n")
	return err
}

// readBlockedUsers reads a list of blocked users from a file (one username per line).
func readBlockedUsers(path string) (map[string]int, error) {
	blocked := make(map[string]int)
	file, err := os.Open(path)
	if err != nil {
		return blocked, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		blocked[line] = 1
	}
	if err := scanner.Err(); err != nil {
		return blocked, err
	}
	return blocked, nil
}

// importTime parses a date string into time.Time using common layouts.
func importTime(dateStr string) (time.Time, error) {
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

// filterRecentPosts filters posts newer than N days ago and not by blocked users.
func filterRecentPosts(posts []Post, days int, blockedUsers map[string]int) []Post {
	now := time.Now().UTC()
	threshold := now.AddDate(0, 0, -days)
	filtered := make([]Post, 0, len(posts))
	for _, post := range posts {
		if _, blocked := blockedUsers[post.Author]; blocked {
			fmt.Printf("Skipping blocked user: %s\n", post.Author)
			continue
		}
		t, err := importTime(post.DateGMT)
		if err != nil {
			continue
		}
		if t.After(threshold) {
			filtered = append(filtered, post)
		}
	}
	return filtered
}

// groupPostsByUser groups posts by their author.
func groupPostsByUser(posts []Post) map[string][]Post {
	userPosts := make(map[string][]Post)
	for _, post := range posts {
		userPosts[post.Author] = append(userPosts[post.Author], post)
	}
	return userPosts
}

type UserTopPost struct {
	User string
	Post Post
}

// getTopPostsByVoteNegative finds the top 1 post with most VoteNegative for each user.
func getTopPostsByVoteNegative(userPosts map[string][]Post) []UserTopPost {
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
	return topPosts
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
