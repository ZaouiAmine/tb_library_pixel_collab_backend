package lib

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/taubyte/go-sdk/database"
	"github.com/taubyte/go-sdk/event"
	http "github.com/taubyte/go-sdk/http/event"
	pubsub "github.com/taubyte/go-sdk/pubsub/node"
)

// ===== TYPES =====
type Pixel struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Color    string `json:"color"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Color    string `json:"color"`
	Online   bool   `json:"online"`
}

type ChatMessage struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// ===== CONSTANTS =====
const (
	CanvasWidth  = 90 // Match frontend canvas size
	CanvasHeight = 90
)

// ===== DEDUPLICATION =====
var processedBatchIds = make(map[string]int64)   // batchId -> timestamp
var processedMessageIds = make(map[string]int64) // messageId -> timestamp
const MAX_PROCESSED_BATCHES = 1000               // Keep last 1000 batch IDs
const MAX_PROCESSED_MESSAGES = 1000              // Keep last 1000 message IDs

// ===== UTILITY FUNCTIONS =====
func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 1
}

// Check if batch ID has already been processed (server-side deduplication)
func isBatchProcessed(batchId string, timestamp int64) bool {
	if batchId == "" {
		return false // No batch ID means we can't deduplicate
	}

	// Check if we've seen this batch ID before
	if _, exists := processedBatchIds[batchId]; exists {
		return true
	}

	// Add this batch ID to processed list
	processedBatchIds[batchId] = timestamp

	// Clean up old batch IDs to prevent memory leaks
	if len(processedBatchIds) > MAX_PROCESSED_BATCHES {
		// Remove oldest entries (simple cleanup - keep last 800)
		count := 0
		for batchId := range processedBatchIds {
			delete(processedBatchIds, batchId)
			count++
			if count >= 200 { // Remove 200 oldest entries
				break
			}
		}
	}

	return false
}

// Check if message ID has already been processed (server-side deduplication)
func isMessageProcessed(messageId string, timestamp int64) bool {
	if messageId == "" {
		return false // No message ID means we can't deduplicate
	}

	// Check if we've seen this message ID before
	if _, exists := processedMessageIds[messageId]; exists {
		return true
	}

	// Add this message ID to processed list
	processedMessageIds[messageId] = timestamp

	// Clean up old message IDs to prevent memory leaks
	if len(processedMessageIds) > MAX_PROCESSED_MESSAGES {
		// Remove oldest entries (simple cleanup - keep last 800)
		count := 0
		for messageId := range processedMessageIds {
			delete(processedMessageIds, messageId)
			count++
			if count >= 200 { // Remove 200 oldest entries
				break
			}
		}
	}

	return false
}

func setCORSHeaders(h http.Event) {
	h.Headers().Set("Access-Control-Allow-Origin", "*")
	h.Headers().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	h.Headers().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// ===== HTTP HANDLERS =====

//export getPixelChannelURL
func getPixelChannelURL(e event.Event) uint32 {

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	setCORSHeaders(h)

	// create/open pixel channel with fixed name
	channel, err := pubsub.Channel("pixelupdates")
	if err != nil {
		return fail(h, err, 500)
	}

	channel.Subscribe()

	// get the websocket url
	url, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, err, 500)
	}

	// Return the WebSocket path directly as a string
	h.Headers().Set("Content-Type", "text/plain")
	h.Write([]byte(url.Path))
	h.Return(200)
	return 0
}

//export getChatChannelURL
func getChatChannelURL(e event.Event) uint32 {

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	setCORSHeaders(h)

	// create/open chat channel with fixed name
	channel, err := pubsub.Channel("chatmessages")
	if err != nil {
		return fail(h, err, 500)
	}

	channel.Subscribe()

	// get the websocket url
	url, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, err, 500)
	}

	// Return the WebSocket path directly as a string
	h.Headers().Set("Content-Type", "text/plain")
	h.Write([]byte(url.Path))
	h.Return(200)
	return 0
}

//export getCanvas
func getCanvas(e event.Event) uint32 {

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	setCORSHeaders(h)

	// get room from query
	room, err := h.Query().Get("room")
	if err != nil {
		return fail(h, err, 400)
	}

	// Open canvas database
	db, err := database.New("/canvas")
	if err != nil {
		return fail(h, err, 500)
	}

	// Create empty canvas
	canvas := make([][]string, CanvasHeight)
	for y := 0; y < CanvasHeight; y++ {
		canvas[y] = make([]string, CanvasWidth)
		for x := 0; x < CanvasWidth; x++ {
			canvas[y][x] = "#ffffff" // White pixels
		}
	}

	// List all keys for this room using CRDT pattern
	roomPrefix := fmt.Sprintf("/%s/", room)
	keys, err := db.List(roomPrefix)
	if err == nil {
		// Process each pixel key
		for _, key := range keys {
			// Parse key to get x,y coordinates
			// Key format: /<room>/<x>:<y>
			if len(key) > len(roomPrefix) {
				coordPart := key[len(roomPrefix):]
				var x, y int
				if n, err := fmt.Sscanf(coordPart, "%d:%d", &x, &y); n == 2 && err == nil {
					if x >= 0 && x < CanvasWidth && y >= 0 && y < CanvasHeight {
						// Get pixel data
						pixelData, err := db.Get(key)
						if err == nil {
							var pixel Pixel
							if json.Unmarshal(pixelData, &pixel) == nil {
								canvas[y][x] = pixel.Color
							}
						}
					}
				}
			}
		}
	}

	// Return reconstructed canvas
	jsonData, err := json.Marshal(canvas)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(jsonData)
	h.Return(200)
	return 0
}

//export clearCanvas
func clearCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// Get room from query parameter
	room, err := h.Query().Get("room")
	if err != nil {
		room = "default"
	}

	// Delete canvas data using CRDT pattern
	db, err := database.New("/canvas")
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}

	// List all pixel keys for this room and delete them
	roomPrefix := fmt.Sprintf("/%s/", room)
	keys, err := db.List(roomPrefix)
	if err == nil {
		for _, key := range keys {
			db.Delete(key)
		}
	}

	h.Write([]byte("Canvas cleared"))
	h.Return(200)
	return 0
}

//export clearChat
func clearChat(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// Get room from query parameter
	room, err := h.Query().Get("room")
	if err != nil {
		room = "default"
	}

	// Delete chat data using CRDT pattern
	db, err := database.New("/chat")
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}

	// List all message keys for this room and delete them
	roomPrefix := fmt.Sprintf("/%s/", room)
	keys, err := db.List(roomPrefix)
	if err == nil {
		for _, key := range keys {
			db.Delete(key)
		}
	}

	h.Write([]byte("Chat cleared"))
	h.Return(200)
	return 0
}

//export getMessages
func getMessages(e event.Event) uint32 {

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	setCORSHeaders(h)

	// get room from query
	room, err := h.Query().Get("room")
	if err != nil {
		return fail(h, err, 400)
	}

	db, err := database.New("/chat")
	if err != nil {
		return fail(h, err, 500)
	}

	// List all message keys for this room using CRDT pattern
	roomPrefix := fmt.Sprintf("/%s/", room)
	keys, err := db.List(roomPrefix)
	if err != nil {
		// Return empty messages array if no data exists
		jsonData, _ := json.Marshal([]ChatMessage{})
		h.Headers().Set("Content-Type", "application/json")
		h.Write(jsonData)
		h.Return(200)
		return 0
	}

	// Collect all messages
	var messages []ChatMessage
	for _, key := range keys {
		// Parse key to get timestamp
		// Key format: /<room>/<timestamp>
		if len(key) > len(roomPrefix) {
			timestampPart := key[len(roomPrefix):]
			var timestamp int64
			if n, err := fmt.Sscanf(timestampPart, "%d", &timestamp); n == 1 && err == nil {
				// Get message data
				messageData, err := db.Get(key)
				if err == nil {
					var message ChatMessage
					if json.Unmarshal(messageData, &message) == nil {
						messages = append(messages, message)
					}
				}
			}
		}
	}

	// Sort messages by timestamp (oldest first)
	for i := 0; i < len(messages); i++ {
		for j := i + 1; j < len(messages); j++ {
			if messages[i].Timestamp > messages[j].Timestamp {
				messages[i], messages[j] = messages[j], messages[i]
			}
		}
	}

	// Keep only last 100 messages
	if len(messages) > 100 {
		messages = messages[len(messages)-100:]
	}

	// Return reconstructed messages
	jsonData, err := json.Marshal(messages)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(jsonData)
	h.Return(200)
	return 0
}

// ===== PUB/SUB HANDLERS =====

//export onPixelUpdate
func onPixelUpdate(e event.Event) uint32 {

	channel, err := e.PubSub()
	if err != nil {
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		return 1
	}

	var pixelBatch struct {
		Pixels    []Pixel `json:"pixels"`
		Room      string  `json:"room"`
		Timestamp int64   `json:"timestamp"`
		BatchId   string  `json:"batchId"`
		SourceId  string  `json:"sourceId"`
	}

	err = json.Unmarshal(data, &pixelBatch)
	if err != nil {
		return 1
	}

	// Check for duplicate batch processing (server-side deduplication)
	if isBatchProcessed(pixelBatch.BatchId, pixelBatch.Timestamp) {
		return 0
	}

	// Use room from message
	room := pixelBatch.Room
	if room == "" {
		room = "default"
	}

	// Update pixels in database using CRDT key pattern
	db, err := database.New("/canvas")
	if err != nil {
		return 1
	}

	// Process each pixel in the batch using CRDT key pattern
	validPixels := []Pixel{}
	for _, pixel := range pixelBatch.Pixels {
		if pixel.X >= 0 && pixel.X < CanvasWidth &&
			pixel.Y >= 0 && pixel.Y < CanvasHeight {

			// Use CRDT key pattern: /<room>/<x>:<y>
			pixelKey := fmt.Sprintf("/%s/%d:%d", room, pixel.X, pixel.Y)

			// Store pixel data as JSON
			pixelData, err := json.Marshal(pixel)
			if err != nil {
				continue
			}

			// Put pixel data in database
			err = db.Put(pixelKey, pixelData)
			if err != nil {
				continue
			}

			validPixels = append(validPixels, pixel)
		}
	}

	return 0
}

//export onChatMessages
func onChatMessages(e event.Event) uint32 {

	channel, err := e.PubSub()
	if err != nil {
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		return 1
	}

	var message struct {
		Message   string `json:"message"`
		UserID    string `json:"userId"`
		Username  string `json:"username"`
		Room      string `json:"room"`
		MessageID string `json:"messageId"`
		Timestamp int64  `json:"timestamp"`
		SourceId  string `json:"sourceId"`
	}

	err = json.Unmarshal(data, &message)
	if err != nil {
		return 1
	}

	// Check for duplicate message processing (server-side deduplication)
	if isMessageProcessed(message.MessageID, message.Timestamp) {
		return 0
	}

	// Use room from message
	room := message.Room
	if room == "" {
		room = "default"
	}

	// Update messages in database using CRDT key pattern
	db, err := database.New("/chat")
	if err != nil {
		return 1
	}

	// Use messageId and timestamp from frontend
	messageId := message.MessageID
	if messageId == "" {
		messageId = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	timestamp := message.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	// Use CRDT key pattern: /<room>/<timestamp>
	chatKey := fmt.Sprintf("/%s/%d", room, timestamp)

	chatMessage := ChatMessage{
		ID:        messageId,
		UserID:    message.UserID,
		Username:  message.Username,
		Message:   message.Message,
		Timestamp: timestamp,
	}

	// Store individual message using CRDT key pattern
	messageData, err := json.Marshal(chatMessage)
	if err != nil {
		return 1
	}

	err = db.Put(chatKey, messageData)
	if err != nil {
		return 1
	}

	// Note: No broadcasting - frontend sends directly to pub/sub for real-time updates

	return 0
}
