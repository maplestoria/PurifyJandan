package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	// The client gets the API key from the environment variable `GEMINI_API_KEY`.
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Download the image.
	imageResp, _ := http.Get("https://goo.gle/instrument-img")

	imageBytes, _ := io.ReadAll(imageResp.Body)

	parts := []*genai.Part{
		genai.NewPartFromBytes(imageBytes, "image/jpeg"),
		genai.NewPartFromText("Caption this image."),
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	result, _ := client.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash",
		contents,
		nil,
	)

	fmt.Println(result.Text())
}
