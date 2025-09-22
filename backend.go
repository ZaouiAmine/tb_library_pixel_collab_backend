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

	// Get canvas data for the room
	canvasKey := "room:" + room
	value, err := db.Get(canvasKey)
	if err != nil {
		// Return empty canvas if no data exists
		emptyCanvas := make([][]string, CanvasHeight)
		for y := 0; y < CanvasHeight; y++ {
			emptyCanvas[y] = make([]string, CanvasWidth)
			for x := 0; x < CanvasWidth; x++ {
				emptyCanvas[y][x] = "#ffffff" // White pixels
			}
		}
		jsonData, _ := json.Marshal(emptyCanvas)
		h.Headers().Set("Content-Type", "application/json")
		h.Write(jsonData)
		h.Return(200)
		return 0
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(value)
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

	// Open canvas database
	db, err := database.New("/canvas")
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}
	defer db.Close()

	// Create white canvas
	whiteCanvas := make([][]string, CanvasHeight)
	for y := 0; y < CanvasHeight; y++ {
		whiteCanvas[y] = make([]string, CanvasWidth)
		for x := 0; x < CanvasWidth; x++ {
			whiteCanvas[y][x] = "#ffffff" // White pixels
		}
	}

	// Save white canvas to database
	canvasData, err := json.Marshal(whiteCanvas)
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}

	// Save to both possible room keys
	db.Put("room:main", canvasData)
	db.Put("room:default", canvasData)

	h.Write([]byte("Canvas cleared - all pixels set to white"))
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

	// Open chat database
	db, err := database.New("/chat")
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}
	defer db.Close()

	// Create empty messages array
	emptyMessages := []ChatMessage{}
	messagesData, err := json.Marshal(emptyMessages)
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}

	// Save empty messages to both possible room keys
	db.Put("room:main", messagesData)
	db.Put("room:default", messagesData)

	h.Write([]byte("Chat cleared - all messages removed"))
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

	chatKey := "room:" + room
	data, err := db.Get(chatKey)
	if err != nil {
		// Return empty messages array if no data exists
		jsonData, _ := json.Marshal([]ChatMessage{})
		h.Headers().Set("Content-Type", "application/json")
		h.Write(jsonData)
		h.Return(200)
		return 0
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(data)
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

	// Update canvas in database
	db, err := database.New("/canvas")
	if err != nil {
		return 1
	}

	canvasKey := "room:" + room
	canvasData, err := db.Get(canvasKey)
	if err != nil {
		// If no canvas exists, create empty canvas
		canvas := make([][]string, CanvasHeight)
		for y := 0; y < CanvasHeight; y++ {
			canvas[y] = make([]string, CanvasWidth)
			for x := 0; x < CanvasWidth; x++ {
				canvas[y][x] = "#ffffff"
			}
		}
		canvasData, _ = json.Marshal(canvas)
		db.Put(canvasKey, canvasData)
	} else {
	}

	var canvas [][]string
	err = json.Unmarshal(canvasData, &canvas)
	if err != nil {
		return 1
	}

	// Process each pixel in the batch
	validPixels := []Pixel{}
	for _, pixel := range pixelBatch.Pixels {
		if pixel.X >= 0 && pixel.X < CanvasWidth &&
			pixel.Y >= 0 && pixel.Y < CanvasHeight {
			canvas[pixel.Y][pixel.X] = pixel.Color
			validPixels = append(validPixels, pixel)
		}
	}

	// Save updated canvas if we have valid pixels
	if len(validPixels) > 0 {
		updatedData, err := json.Marshal(canvas)
		if err != nil {
			return 1
		}
		err = db.Put(canvasKey, updatedData)
		if err != nil {
			return 1
		}

		// Note: No broadcasting - frontend sends directly to pub/sub for real-time updates
	} else {
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

	// Update messages in database
	db, err := database.New("/chat")
	if err != nil {
		return 1
	}

	chatKey := "room:" + room
	messagesData, err := db.Get(chatKey)
	if err != nil {
		messagesData = []byte("[]")
	} else {
	}

	var messages []ChatMessage
	err = json.Unmarshal(messagesData, &messages)
	if err != nil {
		messages = []ChatMessage{}
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

	chatMessage := ChatMessage{
		ID:        messageId,
		UserID:    message.UserID,
		Username:  message.Username,
		Message:   message.Message,
		Timestamp: timestamp,
	}
	messages = append(messages, chatMessage)

	// Keep only last 100 messages
	if len(messages) > 100 {
		messages = messages[len(messages)-100:]
	}

	// Save updated messages
	updatedData, err := json.Marshal(messages)
	if err != nil {
		return 1
	}
	err = db.Put(chatKey, updatedData)
	if err != nil {
		return 1
	}

	// Note: No broadcasting - frontend sends directly to pub/sub for real-time updates

	return 0
}
