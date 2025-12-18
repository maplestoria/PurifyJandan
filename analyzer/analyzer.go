package main

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
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

var allowedUsers = map[string]bool{
	"sein": true,
}

func main() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	parent := filepath.Dir(wd)
	pathBlockedUsers := filepath.Join(parent, "blocked_users.json")
	blockedUsers, err := readBlockedUsers(pathBlockedUsers)
	if err != nil {
		log.Printf("Failed to read blocked users: %v\n", err)
	}
	fmt.Printf("Loaded %d blocked users.\n", len(blockedUsers.Nicknames)+len(blockedUsers.IDs))

	csvPath := filepath.Join(parent, "user_activity.csv")
	fmt.Printf("Reading posts from CSV: %s\n", csvPath)

	posts, err := ReadPostsFromCSV(csvPath)
	if err != nil {
		log.Fatalf("Failed to read posts: %v", err)
	}
	fmt.Printf("Total posts loaded: %d\n", len(posts))

	filtered := filterRecentPosts(posts, 3, blockedUsers)
	fmt.Printf("Posts to be checked in the last 3 days: %d\n", len(filtered))
	userPosts := groupPostsByUser(filtered)
	fmt.Printf("Total users with posts in the last 3 days: %d\n", len(userPosts))
	topPosts := getTopPostsByVoteNegative(userPosts)

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
			if utp.Post.UserId != 0 {
				blockedUsers.IDs = append(blockedUsers.IDs, utp.Post.UserId)
			}
			blockedUsers.Nicknames = append(blockedUsers.Nicknames, utp.Post.Author)
			persistBlockedUser(pathBlockedUsers, blockedUsers)
			fmt.Printf("UserId: %d, Author: %s, Post ID: %d, Image URL: %s is flagged by GenAI analysis.\n", utp.Post.UserId, utp.Post.Author, utp.Post.ID, url)
		} else {
			fmt.Printf("Post ID: %d is clean.\n", utp.Post.ID)
		}
	}
}

// readBlockedUsers reads a list of blocked users from a file (one username per line).
func readBlockedUsers(path string) (BlockedUsers, error) {
	file, err := os.Open(path)
	if err != nil {
		return BlockedUsers{}, err
	}
	defer file.Close()

	var blocked BlockedUsers
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&blocked); err != nil {
		return BlockedUsers{}, err
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
func filterRecentPosts(posts []Post, days int, blockedUsers BlockedUsers) []Post {
	now := time.Now().UTC()
	threshold := now.AddDate(0, 0, -days)
	filtered := make([]Post, 0, len(posts))

	// Build quick lookup sets for blocked user IDs and nicknames
	blockedIDSet := make(map[int]struct{})
	for _, id := range blockedUsers.IDs {
		blockedIDSet[id] = struct{}{}
	}
	blockedNameSet := make(map[string]struct{})
	for _, name := range blockedUsers.Nicknames {
		blockedNameSet[name] = struct{}{}
	}

	for _, post := range posts {
		t, err := importTime(post.DateGMT)
		if err != nil {
			continue
		}
		// Determine if user is blocked by user_id or author
		blocked := false
		if post.UserId != 0 {
			if _, found := blockedIDSet[post.UserId]; found {
				blocked = true
			}
		} else {
			if _, found := blockedNameSet[post.Author]; found {
				blocked = true
			}
		}
		if blocked {
			continue
		}

		userId := strconv.Itoa(post.UserId)
		if t.After(threshold) && (!allowedUsers[post.Author] || (post.UserId != 0 && !allowedUsers[userId])) {
			filtered = append(filtered, post)
		}
	}
	return filtered
}

// groupPostsByUser groups posts by their author.
func groupPostsByUser(posts []Post) map[string][]Post {
	userPosts := make(map[string][]Post)
	for _, post := range posts {
		var key string
		if post.UserId == 0 {
			key = post.Author
		} else {
			key = strconv.Itoa(post.UserId)
		}
		userPosts[key] = append(userPosts[key], post)
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
	// sort topPosts by VoteNegative descending
	for i := 0; i < len(topPosts)-1; i++ {
		for j := i + 1; j < len(topPosts); j++ {
			if topPosts[j].Post.VoteNegative > topPosts[i].Post.VoteNegative {
				topPosts[i], topPosts[j] = topPosts[j], topPosts[i]
			}
		}
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
		genai.NewPartFromText(`Does the image satisfy at least one of the following conditions?
		
			1. Social Media Content or Interactions:
			
				- Screenshots of chat conversations, including:
					- Individuals complaining, venting, or expressing dissatisfaction to others about relationships, work, family, news, or personal matters.
					- People seeking advice, empathy, validation, or support from friends, acquaintances, or the public.
					- Sharing personal experiences, achievements, struggles, or life events in messages or posts.
					- Discussions or debates around current events, news, societal issues, policies, or trending topics.
					- Exchanges involving arguments, heated discussions, or confrontations in group chats, community threads, or comment sections.
					- Posts, tweets, or comments reflecting strong, controversial, or inflammatory opinions (excluding those that are simply humorous, lighthearted, or interesting).
					- Content discussing or responding to memes, viral trends, online challenges, or pop culture phenomena only if the discussion or reaction is likely to provoke dispute, offense, or controversy; ordinary, funny, or non-controversial memes are excluded.
					- Conversations about dating, relationships, boundaries, or social expectations.
					
			2. Gender-Related Themes:
			
				- Depictions, implications, or discussions of gender conflict (such as disagreements or disputes about perspectives, roles, or privileges across genders).
				- Content suggesting entitlement—especially by females—to financial, emotional, or social benefits (including topics like "gold-digging," relationship demands, or debates over gender privilege).
				- Discourse around traditional vs. modern gender roles or stereotypes.
				
			3. Unsettling or Disturbing Content:
			
				- Images featuring animals or creatures commonly associated with fear or discomfort (e.g., snakes, spiders, insects, or other phobia-inducing wildlife).
				- Scenes depicting injuries, medical conditions, graphic, or otherwise distressing content.
				- Unnerving, bizarre, or grotesque visuals designed to provoke a sense of unease.
				
			4. Dispute-Provoking or Controversial Content:
			
				- Content featuring or related to heated debates, arguments, or controversy about political, social, religious, or ideological subjects.
				- Posts, images, or screenshots likely to spark strong emotional reactions (including offensive memes, inflammatory statements, or polarizing opinions).
				- Material promoting misinformation, conspiracy theories, or unfounded claims.
				- Content explicitly designed to provoke, incite arguments, or “troll” others.
		`),
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
	reader.FieldsPerRecord = 7

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
		userId, _ := strconv.Atoi(rec[2])
		voteNeg, _ := strconv.Atoi(rec[4])
		votePos, _ := strconv.Atoi(rec[5])
		content := html.UnescapeString(rec[6])
		post := Post{
			ID:           id,
			Author:       rec[1],
			UserId:       userId,
			DateGMT:      rec[3],
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
	UserId       int
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

// BlockedUsers represents the structure of blocked_users.json
type BlockedUsers struct {
	IDs       []int    `json:"ids"`
	Nicknames []string `json:"nicknames"`
}

// persistBlockedUser saves the blocked users to a JSON file at the given path.
func persistBlockedUser(path string, blocked BlockedUsers) error {
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Failed to open %s for writing: %v", path, err)
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(blocked); err != nil {
		log.Printf("Failed to encode blocked users to %s: %v", path, err)
		return err
	}
	return nil
}
