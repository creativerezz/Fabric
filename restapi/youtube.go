package restapi

import (
	"fmt"
	"log"
	"net/http"

	"github.com/danielmiessler/fabric/common"
	"github.com/danielmiessler/fabric/core"
	"github.com/danielmiessler/fabric/plugins/db/fsdb"
	"github.com/gin-gonic/gin"
	goopenai "github.com/sashabaranov/go-openai"
)

type YouTubeHandler struct {
	registry *core.PluginRegistry
	db       *fsdb.Db
}

type YouTubeRequest struct {
	URL          string `json:"url" binding:"required"`
	Pattern      string `json:"pattern"`
	Language     string `json:"language"`
	WithComments bool   `json:"with_comments"`
	WithMetadata bool   `json:"with_metadata"`
	Model        string `json:"model"`
}

type YouTubeResponse struct {
	Transcript string                 `json:"transcript,omitempty"`
	Comments   []string               `json:"comments,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Pattern    interface{}            `json:"pattern_result,omitempty"`
}

func NewYouTubeHandler(r *gin.RouterGroup, registry *core.PluginRegistry, db *fsdb.Db) *YouTubeHandler {
	handler := &YouTubeHandler{
		registry: registry,
		db:       db,
	}

	r.POST("/youtube", handler.HandleYouTube)
	r.GET("/youtube/:videoId/:pattern", handler.HandleCanonicalYouTube)
	return handler
}

func (h *YouTubeHandler) HandleYouTube(c *gin.Context) {
	var request YouTubeRequest
	if err := c.BindJSON(&request); err != nil {
		log.Printf("Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request format: %v", err)})
		return
	}

	// Set default language if not provided
	if request.Language == "" {
		request.Language = "en"
	}

	// Set default model if not provided and pattern is requested
	if request.Pattern != "" && request.Model == "" {
		request.Model = "gemini-2.0-flash-exp" // Updated default model
	}

	response := YouTubeResponse{}
	errorResponse := make(map[string]interface{})

	// Get video ID
	videoId, _, err := h.registry.YouTube.GetVideoOrPlaylistId(request.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid YouTube URL: %v", err)})
		return
	}

	// Get transcript using simple direct approach like CLI
	transcript, err := h.registry.YouTube.GrabTranscript(videoId, request.Language)
	if err != nil {
		log.Printf("Warning: Failed to get transcript for video %s: %v", videoId, err)
		errorResponse["transcript_error"] = fmt.Sprintf("Failed to get transcript: %v", err)
	} else {
		response.Transcript = transcript
	}

	// If a pattern is specified, process the transcript
	if request.Pattern != "" && transcript != "" {
		// Get chatter with larger context window for Gemini
		// Added "" for the missing 'strategy' argument
		chatter, err := h.registry.GetChatter(request.Model, 128000, "", false, false) // 128K context window
		if err != nil {
			log.Printf("Error creating chatter: %v", err)
			errorResponse["pattern_error"] = fmt.Sprintf("Error creating chatter: %v", err)
		} else {
			// Process entire transcript at once since we have large context window
			chatReq := &common.ChatRequest{
				Message: &goopenai.ChatCompletionMessage{
					Role:    "user",
					Content: transcript,
				},
				PatternName: request.Pattern,
			}

			opts := &common.ChatOptions{
				Model:            request.Model,
				Temperature:      0.7,
				TopP:             0.9,
				FrequencyPenalty: 0.0,
				PresencePenalty:  0.0,
			}

			session, err := chatter.Send(chatReq, opts)
			if err != nil {
				log.Printf("Error processing pattern: %v", err)
				errorResponse["pattern_error"] = fmt.Sprintf("Error processing pattern: %v", err)
			} else if session != nil {
				lastMsg := session.GetLastMessage()
				if lastMsg != nil {
					response.Pattern = lastMsg.Content
				} else {
					errorResponse["pattern_error"] = "No response received from pattern processing"
				}
			}
		}
	}

	// Get comments if requested
	if request.WithComments {
		comments, err := h.registry.YouTube.GrabComments(videoId)
		if err != nil {
			log.Printf("Warning: Failed to get comments: %v", err)
			errorResponse["comments_error"] = fmt.Sprintf("Failed to get comments: %v", err)
		} else {
			response.Comments = comments
		}
	}

	// Get metadata if requested
	if request.WithMetadata {
		metadata, err := h.registry.YouTube.GrabMetadata(videoId)
		if err != nil {
			log.Printf("Warning: Failed to get metadata: %v", err)
			errorResponse["metadata_error"] = fmt.Sprintf("Failed to get metadata: %v", err)
		} else {
			// Convert metadata to map
			response.Metadata = map[string]interface{}{
				"id":           metadata.Id,
				"title":        metadata.Title,
				"description":  metadata.Description,
				"publishedAt":  metadata.PublishedAt,
				"channelId":    metadata.ChannelId,
				"channelTitle": metadata.ChannelTitle,
				"categoryId":   metadata.CategoryId,
				"tags":         metadata.Tags,
				"viewCount":    metadata.ViewCount,
				"likeCount":    metadata.LikeCount,
			}
		}
	}

	// Combine response with any errors
	result := make(map[string]interface{})

	// Add successful response data
	if response.Transcript != "" {
		result["transcript"] = response.Transcript
	}
	if response.Pattern != nil {
		result["pattern_result"] = response.Pattern
	}
	if len(response.Comments) > 0 {
		result["comments"] = response.Comments
	}
	if response.Metadata != nil {
		result["metadata"] = response.Metadata
	}

	// Add any errors
	if len(errorResponse) > 0 {
		result["errors"] = errorResponse
	}

	c.JSON(http.StatusOK, result)
}

// HandleCanonicalYouTube processes a YouTube video with a pattern using URL parameters
func (h *YouTubeHandler) HandleCanonicalYouTube(c *gin.Context) {
	videoId := c.Param("videoId")
	pattern := c.Param("pattern")

	// Construct the YouTube URL
	url := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoId)

	// Create a request object with the extracted parameters
	request := YouTubeRequest{
		URL:      url,
		Pattern:  pattern,
		Language: "en",                   // Default language
		Model:    "gemini-2.0-flash-exp", // Default model
	}

	// Process using the same logic as the POST endpoint
	response := YouTubeResponse{}
	errorResponse := make(map[string]interface{})

	// Get transcript using simple direct approach like CLI
	transcript, err := h.registry.YouTube.GrabTranscript(videoId, request.Language)
	if err != nil {
		log.Printf("Warning: Failed to get transcript for video %s: %v", videoId, err)
		errorResponse["transcript_error"] = fmt.Sprintf("Failed to get transcript: %v", err)
	} else {
		response.Transcript = transcript
	}

	// If a pattern is specified, process the transcript
	if request.Pattern != "" && transcript != "" {
		// Get chatter with larger context window for Gemini
		// Added "" for the missing 'strategy' argument
		chatter, err := h.registry.GetChatter(request.Model, 128000, "", false, false) // 128K context window
		if err != nil {
			log.Printf("Error creating chatter: %v", err)
			errorResponse["pattern_error"] = fmt.Sprintf("Error creating chatter: %v", err)
		} else {
			// Process entire transcript at once since we have large context window
			chatReq := &common.ChatRequest{
				Message: &goopenai.ChatCompletionMessage{
					Role:    "user",
					Content: transcript,
				},
				PatternName: request.Pattern,
			}

			opts := &common.ChatOptions{
				Model:            request.Model,
				Temperature:      0.7,
				TopP:             0.9,
				FrequencyPenalty: 0.0,
				PresencePenalty:  0.0,
			}

			session, err := chatter.Send(chatReq, opts)
			if err != nil {
				log.Printf("Error processing pattern: %v", err)
				errorResponse["pattern_error"] = fmt.Sprintf("Error processing pattern: %v", err)
			} else if session != nil {
				lastMsg := session.GetLastMessage()
				if lastMsg != nil {
					response.Pattern = lastMsg.Content
				} else {
					errorResponse["pattern_error"] = "No response received from pattern processing"
				}
			}
		}
	}

	// Combine response with any errors
	result := make(map[string]interface{})

	// Add successful response data
	if response.Transcript != "" {
		result["transcript"] = response.Transcript
	}
	if response.Pattern != nil {
		result["pattern_result"] = response.Pattern
	}

	// Add any errors
	if len(errorResponse) > 0 {
		result["errors"] = errorResponse
	}

	c.JSON(http.StatusOK, result)
}
