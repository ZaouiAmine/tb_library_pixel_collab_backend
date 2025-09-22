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
	if existingTimestamp, exists := processedBatchIds[batchId]; exists {
		fmt.Printf("🔄 [DEDUP] Batch ID '%s' already processed at timestamp %d, ignoring\n", batchId, existingTimestamp)
		return true
	}

	// Add this batch ID to processed list
	processedBatchIds[batchId] = timestamp
	fmt.Printf("✅ [DEDUP] Batch ID '%s' marked as processed at timestamp %d\n", batchId, timestamp)

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
		fmt.Printf("🧹 [DEDUP] Cleaned up old batch IDs, remaining: %d\n", len(processedBatchIds))
	}

	return false
}

// Check if message ID has already been processed (server-side deduplication)
func isMessageProcessed(messageId string, timestamp int64) bool {
	if messageId == "" {
		return false // No message ID means we can't deduplicate
	}

	// Check if we've seen this message ID before
	if existingTimestamp, exists := processedMessageIds[messageId]; exists {
		fmt.Printf("🔄 [DEDUP] Message ID '%s' already processed at timestamp %d, ignoring\n", messageId, existingTimestamp)
		return true
	}

	// Add this message ID to processed list
	processedMessageIds[messageId] = timestamp
	fmt.Printf("✅ [DEDUP] Message ID '%s' marked as processed at timestamp %d\n", messageId, timestamp)

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
		fmt.Printf("🧹 [DEDUP] Cleaned up old message IDs, remaining: %d\n", len(processedMessageIds))
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
	fmt.Println("🔗 [getPixelChannelURL] Starting pixel channel URL request")

	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("❌ [getPixelChannelURL] Error getting HTTP event: %v\n", err)
		return 1
	}
	fmt.Println("✅ [getPixelChannelURL] HTTP event obtained successfully")

	setCORSHeaders(h)
	fmt.Println("🌐 [getPixelChannelURL] CORS headers set")

	// create/open pixel channel with fixed name
	fmt.Println("📡 [getPixelChannelURL] Creating pixel channel 'pixelupdates'")
	channel, err := pubsub.Channel("pixelupdates")
	if err != nil {
		fmt.Printf("❌ [getPixelChannelURL] Error creating channel: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Println("✅ [getPixelChannelURL] Pixel channel created successfully")

	fmt.Println("🔔 [getPixelChannelURL] Subscribing to pixel channel")
	channel.Subscribe()
	fmt.Println("✅ [getPixelChannelURL] Successfully subscribed to pixel channel")

	// get the websocket url
	fmt.Println("🔗 [getPixelChannelURL] Getting WebSocket URL")
	url, err := channel.WebSocket().Url()
	if err != nil {
		fmt.Printf("❌ [getPixelChannelURL] Error getting WebSocket URL: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Printf("✅ [getPixelChannelURL] WebSocket URL obtained: %s\n", url.Path)

	// Return the WebSocket path directly as a string
	h.Headers().Set("Content-Type", "text/plain")
	h.Write([]byte(url.Path))
	h.Return(200)
	fmt.Println("🎉 [getPixelChannelURL] Successfully returned pixel channel URL")
	return 0
}

//export getChatChannelURL
func getChatChannelURL(e event.Event) uint32 {
	fmt.Println("💬 [getChatChannelURL] Starting chat channel URL request")

	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("❌ [getChatChannelURL] Error getting HTTP event: %v\n", err)
		return 1
	}
	fmt.Println("✅ [getChatChannelURL] HTTP event obtained successfully")

	setCORSHeaders(h)
	fmt.Println("🌐 [getChatChannelURL] CORS headers set")

	// create/open chat channel with fixed name
	fmt.Println("📡 [getChatChannelURL] Creating chat channel 'chatmessages'")
	channel, err := pubsub.Channel("chatmessages")
	if err != nil {
		fmt.Printf("❌ [getChatChannelURL] Error creating channel: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Println("✅ [getChatChannelURL] Chat channel created successfully")

	fmt.Println("🔔 [getChatChannelURL] Subscribing to chat channel")
	channel.Subscribe()
	fmt.Println("✅ [getChatChannelURL] Successfully subscribed to chat channel")

	// get the websocket url
	fmt.Println("🔗 [getChatChannelURL] Getting WebSocket URL")
	url, err := channel.WebSocket().Url()
	if err != nil {
		fmt.Printf("❌ [getChatChannelURL] Error getting WebSocket URL: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Printf("✅ [getChatChannelURL] WebSocket URL obtained: %s\n", url.Path)

	// Return the WebSocket path directly as a string
	h.Headers().Set("Content-Type", "text/plain")
	h.Write([]byte(url.Path))
	h.Return(200)
	fmt.Println("🎉 [getChatChannelURL] Successfully returned chat channel URL")
	return 0
}

//export getCanvas
func getCanvas(e event.Event) uint32 {
	fmt.Println("🎨 [getCanvas] Starting canvas data request")

	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("❌ [getCanvas] Error getting HTTP event: %v\n", err)
		return 1
	}
	fmt.Println("✅ [getCanvas] HTTP event obtained successfully")

	setCORSHeaders(h)
	fmt.Println("🌐 [getCanvas] CORS headers set")

	// get room from query
	fmt.Println("🏠 [getCanvas] Getting room parameter from query")
	room, err := h.Query().Get("room")
	if err != nil {
		fmt.Printf("❌ [getCanvas] Error getting room parameter: %v\n", err)
		return fail(h, err, 400)
	}
	fmt.Printf("✅ [getCanvas] Room parameter obtained: '%s'\n", room)

	// Open canvas database
	fmt.Println("💾 [getCanvas] Opening canvas database")
	db, err := database.New("/canvas")

	if err != nil {
		fmt.Printf("❌ [getCanvas] Error opening database: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Println("✅ [getCanvas] Canvas database opened successfully")

	// Get canvas data for the room
	canvasKey := "room:" + room
	fmt.Printf("🔑 [getCanvas] Looking for canvas data with key: '%s'\n", canvasKey)
	value, err := db.Get(canvasKey)
	if err != nil {
		fmt.Printf("⚠️ [getCanvas] No canvas data found for room '%s', creating empty canvas\n", room)
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
		fmt.Printf("✅ [getCanvas] Returned empty canvas (%dx%d) for room '%s'\n", CanvasWidth, CanvasHeight, room)
		return 0
	}

	fmt.Printf("✅ [getCanvas] Canvas data found for room '%s' (%d bytes)\n", room, len(value))
	h.Headers().Set("Content-Type", "application/json")
	h.Write(value)
	h.Return(200)
	fmt.Printf("🎉 [getCanvas] Successfully returned canvas data for room '%s'\n", room)
	return 0
}

//export clearCanvas
func clearCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// Get room parameter from query (same logic as other functions)
	room, err := h.Query().Get("room")
	if err != nil || room == "" {
		room = "default"
	}

	// Open canvas database
	db, err := database.New("/canvas")
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}
	defer db.Close()

	// Delete canvas data for the room
	canvasKey := "room:" + room
	err = db.Delete(canvasKey)
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}

	h.Write([]byte(fmt.Sprintf("Canvas cleared for room: %s", room)))
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

	// Get room parameter from query (same logic as other functions)
	room, err := h.Query().Get("room")
	if err != nil || room == "" {
		room = "default"
	}

	// Open chat database
	db, err := database.New("/chat")
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}
	defer db.Close()

	// Delete chat data for the room
	chatKey := "room:" + room
	err = db.Delete(chatKey)
	if err != nil {
		h.Write([]byte(fmt.Sprintf("Error: %v", err)))
		h.Return(500)
		return 1
	}

	h.Write([]byte(fmt.Sprintf("Chat cleared for room: %s", room)))
	h.Return(200)
	return 0
}

//export getMessages
func getMessages(e event.Event) uint32 {
	fmt.Println("💬 [getMessages] Starting chat messages request")

	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("❌ [getMessages] Error getting HTTP event: %v\n", err)
		return 1
	}
	fmt.Println("✅ [getMessages] HTTP event obtained successfully")

	setCORSHeaders(h)
	fmt.Println("🌐 [getMessages] CORS headers set")

	// get room from query
	fmt.Println("🏠 [getMessages] Getting room parameter from query")
	room, err := h.Query().Get("room")
	if err != nil {
		fmt.Printf("❌ [getMessages] Error getting room parameter: %v\n", err)
		return fail(h, err, 400)
	}
	fmt.Printf("✅ [getMessages] Room parameter obtained: '%s'\n", room)

	fmt.Println("💾 [getMessages] Opening chat database")
	db, err := database.New("/chat")
	if err != nil {
		fmt.Printf("❌ [getMessages] Error opening chat database: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Println("✅ [getMessages] Chat database opened successfully")

	chatKey := "room:" + room
	fmt.Printf("🔑 [getMessages] Looking for chat data with key: '%s'\n", chatKey)
	data, err := db.Get(chatKey)
	if err != nil {
		fmt.Printf("⚠️ [getMessages] No chat data found for room '%s', returning empty array\n", room)
		// Return empty messages array if no data exists
		jsonData, _ := json.Marshal([]ChatMessage{})
		h.Headers().Set("Content-Type", "application/json")
		h.Write(jsonData)
		h.Return(200)
		fmt.Printf("✅ [getMessages] Returned empty chat array for room '%s'\n", room)
		return 0
	}

	fmt.Printf("✅ [getMessages] Chat data found for room '%s' (%d bytes)\n", room, len(data))
	h.Headers().Set("Content-Type", "application/json")
	h.Write(data)
	h.Return(200)
	fmt.Printf("🎉 [getMessages] Successfully returned chat data for room '%s'\n", room)
	return 0
}

// ===== PUB/SUB HANDLERS =====

//export onPixelUpdate
func onPixelUpdate(e event.Event) uint32 {
	fmt.Println("🎨 [onPixelUpdate] ===== PIXEL UPDATE HANDLER TRIGGERED =====")
	fmt.Printf("📊 [onPixelUpdate] Event type: %T\n", e)

	fmt.Println("📡 [onPixelUpdate] Getting PubSub channel")
	channel, err := e.PubSub()
	if err != nil {
		fmt.Printf("❌ [onPixelUpdate] Error getting PubSub channel: %v\n", err)
		return 1
	}
	fmt.Println("✅ [onPixelUpdate] PubSub channel obtained successfully")

	fmt.Println("📦 [onPixelUpdate] Getting channel data")
	data, err := channel.Data()
	if err != nil {
		fmt.Printf("❌ [onPixelUpdate] Error getting channel data: %v\n", err)
		return 1
	}
	fmt.Printf("✅ [onPixelUpdate] Channel data obtained (%d bytes)\n", len(data))
	fmt.Printf("📄 [onPixelUpdate] Raw data: %s\n", string(data))

	var pixelBatch struct {
		Pixels    []Pixel `json:"pixels"`
		Room      string  `json:"room"`
		Timestamp int64   `json:"timestamp"`
		BatchId   string  `json:"batchId"`
		SourceId  string  `json:"sourceId"`
	}

	fmt.Println("🔍 [onPixelUpdate] Parsing pixel batch data")
	err = json.Unmarshal(data, &pixelBatch)
	if err != nil {
		fmt.Printf("❌ [onPixelUpdate] Error parsing pixel batch data: %v\n", err)
		return 1
	}

	fmt.Printf("✅ [onPixelUpdate] Successfully parsed batch: %d pixels, room='%s', timestamp=%d, batchId='%s', sourceId='%s'\n",
		len(pixelBatch.Pixels), pixelBatch.Room, pixelBatch.Timestamp, pixelBatch.BatchId, pixelBatch.SourceId)

	// Check for duplicate batch processing (server-side deduplication)
	if isBatchProcessed(pixelBatch.BatchId, pixelBatch.Timestamp) {
		fmt.Println("🚫 [onPixelUpdate] Duplicate batch detected, skipping processing")
		return 0
	}

	// Use room from message
	room := pixelBatch.Room
	if room == "" {
		room = "default"
		fmt.Printf("⚠️ [onPixelUpdate] Empty room detected, using default room\n")
	}
	fmt.Printf("🏠 [onPixelUpdate] Processing pixels for room: '%s'\n", room)

	// Update canvas in database
	fmt.Println("💾 [onPixelUpdate] Opening canvas database")
	db, err := database.New("/canvas")
	if err != nil {
		fmt.Printf("❌ [onPixelUpdate] Error opening canvas database: %v\n", err)
		return 1
	}
	fmt.Println("✅ [onPixelUpdate] Canvas database opened successfully")

	canvasKey := "room:" + room
	fmt.Printf("🔑 [onPixelUpdate] Getting canvas data with key: '%s'\n", canvasKey)
	canvasData, err := db.Get(canvasKey)
	if err != nil {
		fmt.Printf("⚠️ [onPixelUpdate] No canvas found for room '%s', creating new canvas\n", room)
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
		fmt.Printf("✅ [onPixelUpdate] Created new empty canvas (%dx%d)\n", CanvasWidth, CanvasHeight)
	} else {
		fmt.Printf("✅ [onPixelUpdate] Found existing canvas data (%d bytes)\n", len(canvasData))
	}

	fmt.Println("🔍 [onPixelUpdate] Parsing canvas data")
	var canvas [][]string
	err = json.Unmarshal(canvasData, &canvas)
	if err != nil {
		fmt.Printf("❌ [onPixelUpdate] Error parsing canvas data: %v\n", err)
		return 1
	}
	fmt.Printf("✅ [onPixelUpdate] Canvas parsed successfully (%dx%d)\n", len(canvas[0]), len(canvas))

	// Process each pixel in the batch
	fmt.Printf("🎯 [onPixelUpdate] Processing %d pixels in batch\n", len(pixelBatch.Pixels))
	validPixels := []Pixel{}
	for i, pixel := range pixelBatch.Pixels {
		fmt.Printf("📍 [onPixelUpdate] Pixel %d/%d: x=%d, y=%d, color=%s, user=%s\n",
			i+1, len(pixelBatch.Pixels), pixel.X, pixel.Y, pixel.Color, pixel.Username)

		if pixel.X >= 0 && pixel.X < CanvasWidth &&
			pixel.Y >= 0 && pixel.Y < CanvasHeight {
			canvas[pixel.Y][pixel.X] = pixel.Color
			validPixels = append(validPixels, pixel)
			fmt.Printf("✅ [onPixelUpdate] Pixel updated: (%d,%d) = %s\n", pixel.X, pixel.Y, pixel.Color)
		} else {
			fmt.Printf("❌ [onPixelUpdate] Pixel out of bounds: (%d,%d) - canvas size: %dx%d\n",
				pixel.X, pixel.Y, CanvasWidth, CanvasHeight)
		}
	}
	fmt.Printf("📊 [onPixelUpdate] Processed %d valid pixels out of %d total\n", len(validPixels), len(pixelBatch.Pixels))

	// Save updated canvas if we have valid pixels
	if len(validPixels) > 0 {
		fmt.Println("💾 [onPixelUpdate] Saving updated canvas to database")
		updatedData, err := json.Marshal(canvas)
		if err != nil {
			fmt.Printf("❌ [onPixelUpdate] Error marshaling canvas data: %v\n", err)
			return 1
		}
		err = db.Put(canvasKey, updatedData)
		if err != nil {
			fmt.Printf("❌ [onPixelUpdate] Error saving canvas to database: %v\n", err)
			return 1
		}
		fmt.Printf("✅ [onPixelUpdate] Canvas saved successfully (%d bytes)\n", len(updatedData))

		// Note: No broadcasting - frontend sends directly to pub/sub for real-time updates
		fmt.Println("✅ [onPixelUpdate] Pixel batch saved to database (no broadcasting needed)")
	} else {
		fmt.Println("⚠️ [onPixelUpdate] No valid pixels to process, skipping save and broadcast")
	}

	fmt.Println("🎉 [onPixelUpdate] ===== PIXEL UPDATE HANDLER COMPLETED =====")
	return 0
}

//export onChatMessages
func onChatMessages(e event.Event) uint32 {
	fmt.Println("💬 [onChatMessage] ===== CHAT MESSAGE HANDLER TRIGGERED =====")
	fmt.Printf("📊 [onChatMessage] Event type: %T\n", e)

	fmt.Println("📡 [onChatMessage] Getting PubSub channel")
	channel, err := e.PubSub()
	if err != nil {
		fmt.Printf("❌ [onChatMessage] Error getting PubSub channel: %v\n", err)
		return 1
	}
	fmt.Println("✅ [onChatMessage] PubSub channel obtained successfully")

	fmt.Println("📦 [onChatMessage] Getting channel data")
	data, err := channel.Data()
	if err != nil {
		fmt.Printf("❌ [onChatMessage] Error getting channel data: %v\n", err)
		return 1
	}
	fmt.Printf("✅ [onChatMessage] Channel data obtained (%d bytes)\n", len(data))
	fmt.Printf("📄 [onChatMessage] Raw data: %s\n", string(data))

	var message struct {
		Message   string `json:"message"`
		UserID    string `json:"userId"`
		Username  string `json:"username"`
		Room      string `json:"room"`
		MessageID string `json:"messageId"`
		Timestamp int64  `json:"timestamp"`
		SourceId  string `json:"sourceId"`
	}

	fmt.Println("🔍 [onChatMessage] Parsing chat message data")
	err = json.Unmarshal(data, &message)
	if err != nil {
		fmt.Printf("❌ [onChatMessage] Error parsing chat message data: %v\n", err)
		return 1
	}

	fmt.Printf("✅ [onChatMessage] Successfully parsed message: user='%s', message='%s', room='%s', messageId='%s', timestamp=%d, sourceId='%s'\n",
		message.Username, message.Message, message.Room, message.MessageID, message.Timestamp, message.SourceId)

	// Check for duplicate message processing (server-side deduplication)
	if isMessageProcessed(message.MessageID, message.Timestamp) {
		fmt.Println("🚫 [onChatMessage] Duplicate message detected, skipping processing")
		return 0
	}

	// Use room from message
	room := message.Room
	if room == "" {
		room = "default"
		fmt.Printf("⚠️ [onChatMessage] Empty room detected, using default room\n")
	}
	fmt.Printf("🏠 [onChatMessage] Processing chat message for room: '%s'\n", room)

	// Update messages in database
	fmt.Println("💾 [onChatMessage] Opening chat database")
	db, err := database.New("/chat")
	if err != nil {
		fmt.Printf("❌ [onChatMessage] Error opening chat database: %v\n", err)
		return 1
	}
	fmt.Println("✅ [onChatMessage] Chat database opened successfully")

	chatKey := "room:" + room
	fmt.Printf("🔑 [onChatMessage] Getting chat data with key: '%s'\n", chatKey)
	messagesData, err := db.Get(chatKey)
	if err != nil {
		fmt.Printf("⚠️ [onChatMessage] No chat data found for room '%s', initializing empty array\n", room)
		messagesData = []byte("[]")
	} else {
		fmt.Printf("✅ [onChatMessage] Found existing chat data (%d bytes)\n", len(messagesData))
	}

	fmt.Println("🔍 [onChatMessage] Parsing existing messages")
	var messages []ChatMessage
	err = json.Unmarshal(messagesData, &messages)
	if err != nil {
		fmt.Printf("⚠️ [onChatMessage] Error parsing messages, initializing empty array: %v\n", err)
		messages = []ChatMessage{}
	}
	fmt.Printf("📊 [onChatMessage] Loaded %d existing messages\n", len(messages))

	// Use messageId and timestamp from frontend
	messageId := message.MessageID
	if messageId == "" {
		messageId = fmt.Sprintf("%d", time.Now().UnixNano())
		fmt.Printf("⚠️ [onChatMessage] No messageId from frontend, generated: %s\n", messageId)
	}

	timestamp := message.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().Unix()
		fmt.Printf("⚠️ [onChatMessage] No timestamp from frontend, generated: %d\n", timestamp)
	}

	chatMessage := ChatMessage{
		ID:        messageId,
		UserID:    message.UserID,
		Username:  message.Username,
		Message:   message.Message,
		Timestamp: timestamp,
	}
	messages = append(messages, chatMessage)
	fmt.Printf("✅ [onChatMessage] Added new message: ID=%s, user=%s, timestamp=%d\n",
		messageId, message.Username, timestamp)

	// Keep only last 100 messages
	if len(messages) > 100 {
		removed := len(messages) - 100
		messages = messages[len(messages)-100:]
		fmt.Printf("🧹 [onChatMessage] Trimmed %d old messages, keeping last 100\n", removed)
	}
	fmt.Printf("📊 [onChatMessage] Total messages in room: %d\n", len(messages))

	// Save updated messages
	fmt.Println("💾 [onChatMessage] Saving updated messages to database")
	updatedData, err := json.Marshal(messages)
	if err != nil {
		fmt.Printf("❌ [onChatMessage] Error marshaling messages: %v\n", err)
		return 1
	}
	err = db.Put(chatKey, updatedData)
	if err != nil {
		fmt.Printf("❌ [onChatMessage] Error saving messages to database: %v\n", err)
		return 1
	}
	fmt.Printf("✅ [onChatMessage] Messages saved successfully (%d bytes)\n", len(updatedData))

	// Note: No broadcasting - frontend sends directly to pub/sub for real-time updates
	fmt.Println("✅ [onChatMessage] Chat message saved to database (no broadcasting needed)")

	fmt.Println("🎉 [onChatMessage] ===== CHAT MESSAGE HANDLER COMPLETED =====")
	return 0
}
