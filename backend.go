package lib

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/taubyte/go-sdk/database"
	"github.com/taubyte/go-sdk/event"
	http "github.com/taubyte/go-sdk/http/event"
	pubsub "github.com/taubyte/go-sdk/pubsub/node"
)

// ===== Constants =====
const (
	CanvasWidth  = 100
	CanvasHeight = 100
	MaxMessages  = 100
	UserTimeout  = 30 * time.Second
)

// ===== Data Structures =====

type Pixel struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Color     string `json:"color"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Timestamp int64  `json:"timestamp"`
}

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Color        string `json:"color"`
	IsOnline     bool   `json:"isOnline"`
	LastSeen     int64  `json:"lastSeen"`
	PixelsPlaced int    `json:"pixelsPlaced"`
}

type ChatMessage struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

type ChatMessageRequest struct {
	Message string `json:"message"`
}

// ===== Global State =====
var (
	canvasCache [][]Pixel = make([][]Pixel, 0)
	cacheMutex  sync.RWMutex
	cacheValid  bool
)

// ===== Utility Functions =====

func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 0
}

func isValidUsername(username string) bool {
	return len(username) >= 1 && len(username) <= 20 && !strings.ContainsAny(username, "<>\"'&")
}

func isValidColor(color string) bool {
	return len(color) == 7 && color[0] == '#' &&
		strings.ContainsAny(color[1:], "0123456789ABCDEFabcdef")
}

func sanitizeMessage(message string) string {
	// Remove script tags and dangerous attributes
	message = strings.ReplaceAll(message, "<script", "")
	message = strings.ReplaceAll(message, "</script>", "")
	message = strings.ReplaceAll(message, "javascript:", "")
	message = strings.ReplaceAll(message, "onload=", "")
	message = strings.ReplaceAll(message, "onerror=", "")
	message = strings.ReplaceAll(message, "onclick=", "")

	// Limit length
	if len(message) > 500 {
		message = message[:500]
	}

	return strings.TrimSpace(message)
}

// ===== Database Operations =====

func getDB() (database.Database, error) {
	db, err := database.New("/canvas")
	if err != nil {
		fmt.Printf("Error creating database connection: %v\n", err)
		return db, err
	}
	return db, nil
}

func createCanvas() error {
	db, err := getDB()
	if err != nil {
		return err
	}

	// Always create fresh canvas
	canvas := make([][]Pixel, CanvasHeight)
	for y := range canvas {
		canvas[y] = make([]Pixel, CanvasWidth)
		for x := range canvas[y] {
			canvas[y][x] = Pixel{
				X:         x,
				Y:         y,
				Color:     "#ffffff",
				UserID:    "",
				Username:  "",
				Timestamp: 0,
			}
		}
	}

	canvasData, err := json.Marshal(canvas)
	if err != nil {
		return err
	}

	return db.Put("canvas", canvasData)
}

func getCanvasFromDB() ([][]Pixel, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	// Try to get existing canvas
	data, err := db.Get("canvas")
	if err != nil {
		// Canvas doesn't exist, create it
		if err := createCanvas(); err != nil {
			return nil, err
		}
		data, err = db.Get("canvas")
		if err != nil {
			return nil, err
		}
	}

	// Unmarshal canvas data
	var canvas [][]Pixel
	if err := json.Unmarshal(data, &canvas); err != nil {
		// Data is corrupted, recreate canvas
		if err := createCanvas(); err != nil {
			return nil, err
		}
		data, err = db.Get("canvas")
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &canvas); err != nil {
			return nil, err
		}
	}

	// Update cache
	cacheMutex.Lock()
	canvasCache = canvas
	cacheValid = true
	cacheMutex.Unlock()

	return canvas, nil
}

func savePixelToDB(pixel Pixel) error {
	db, err := getDB()
	if err != nil {
		return err
	}

	// Get current canvas
	canvas, err := getCanvasFromDB()
	if err != nil {
		return err
	}

	// Update pixel
	if pixel.Y < len(canvas) && pixel.X < len(canvas[pixel.Y]) {
		canvas[pixel.Y][pixel.X] = pixel
	}

	// Save back to database
	canvasData, err := json.Marshal(canvas)
	if err != nil {
		return err
	}

	// Update cache
	cacheMutex.Lock()
	canvasCache = canvas
	cacheMutex.Unlock()

	return db.Put("canvas", canvasData)
}

func getUsersFromDB() ([]User, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	data, err := db.Get("users")
	if err != nil {
		return []User{}, nil // Return empty list if no users
	}

	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return []User{}, nil // Return empty list if corrupted
	}

	// Filter out offline users
	now := time.Now().UnixMilli()
	var activeUsers []User
	for _, user := range users {
		if user.IsOnline && (now-user.LastSeen) < int64(UserTimeout.Milliseconds()) {
			activeUsers = append(activeUsers, user)
		}
	}

	return activeUsers, nil
}

func saveUserToDB(user User) error {
	db, err := getDB()
	if err != nil {
		return err
	}

	// Get existing users
	users, err := getUsersFromDB()
	if err != nil {
		users = []User{}
	}

	// Update or add user
	found := false
	for i, u := range users {
		if u.ID == user.ID {
			users[i] = user
			found = true
			break
		}
	}
	if !found {
		users = append(users, user)
	}

	// Save back to database
	usersData, err := json.Marshal(users)
	if err != nil {
		return err
	}

	return db.Put("users", usersData)
}

func getChatMessagesFromDB() ([]ChatMessage, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	data, err := db.Get("messages")
	if err != nil {
		return []ChatMessage{}, nil // Return empty list if no messages
	}

	var messages []ChatMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return []ChatMessage{}, nil // Return empty list if corrupted
	}

	// Sort by timestamp (newest first) and limit
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp > messages[j].Timestamp
	})

	if len(messages) > MaxMessages {
		messages = messages[:MaxMessages]
	}

	return messages, nil
}

func saveChatMessageToDB(message ChatMessage) error {
	db, err := getDB()
	if err != nil {
		return err
	}

	// Get existing messages
	messages, err := getChatMessagesFromDB()
	if err != nil {
		messages = []ChatMessage{}
	}

	// Add new message
	messages = append(messages, message)

	// Sort by timestamp (newest first) and limit
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp > messages[j].Timestamp
	})

	if len(messages) > MaxMessages {
		messages = messages[:MaxMessages]
	}

	// Save back to database
	messagesData, err := json.Marshal(messages)
	if err != nil {
		return err
	}

	return db.Put("messages", messagesData)
}

// ===== Pub/Sub Functions =====

func publishPixelUpdate(pixel Pixel) error {
	channel, err := pubsub.Channel("pixelupdates")
	if err != nil {
		return err
	}

	pixelData, err := json.Marshal(pixel)
	if err != nil {
		return err
	}

	return channel.Publish(pixelData)
}

func publishUserUpdate(user User) error {
	channel, err := pubsub.Channel("userupdates")
	if err != nil {
		return err
	}

	userData, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return channel.Publish(userData)
}

func publishChatMessage(message ChatMessage) error {
	channel, err := pubsub.Channel("chatmessages")
	if err != nil {
		return err
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return channel.Publish(messageData)
}

// ===== HTTP Handlers =====

//export getCanvas
func getCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("Error getting HTTP handler in getCanvas: %v\n", err)
		return 1
	}

	// Try cache first
	cacheMutex.RLock()
	if cacheValid && len(canvasCache) > 0 {
		canvasData, err := json.Marshal(canvasCache)
		cacheMutex.RUnlock()
		if err == nil {
			h.Headers().Set("Content-Type", "application/json")
			h.Write(canvasData)
			h.Return(200)
			return 0
		}
	}
	cacheMutex.RUnlock()

	// Get from database
	canvas, err := getCanvasFromDB()
	if err != nil {
		fmt.Printf("Error getting canvas from database: %v\n", err)
		return fail(h, err, 500)
	}

	canvasData, err := json.Marshal(canvas)
	if err != nil {
		fmt.Printf("Error marshaling canvas data: %v\n", err)
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(canvasData)
	h.Return(200)
	return 0
}

//export getUsers
func getUsers(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	users, err := getUsersFromDB()
	if err != nil {
		return fail(h, err, 500)
	}

	usersData, err := json.Marshal(users)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(usersData)
	h.Return(200)
	return 0
}

//export getMessages
func getMessages(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	messages, err := getChatMessagesFromDB()
	if err != nil {
		return fail(h, err, 500)
	}

	messagesData, err := json.Marshal(messages)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(messagesData)
	h.Return(200)
	return 0
}

//export getWebSocketURL
func getWebSocketURL(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get room parameter from query string
	room, err := h.Query().Get("room")
	if err != nil || room == "" {
		room = "pixelupdates"
	}

	// Validate room name
	allowedChannels := map[string]bool{
		"pixelupdates": true,
		"chatmessages": true,
		"userupdates":  true,
	}

	if !allowedChannels[room] {
		return fail(h, fmt.Errorf("invalid room name: %s", room), 400)
	}

	// Get the pub/sub channel
	fmt.Printf("Attempting to get channel: %s\n", room)
	channel, err := pubsub.Channel(room)
	if err != nil {
		fmt.Printf("Error getting channel %s: %v\n", room, err)
		return fail(h, fmt.Errorf("failed to get channel %s: %v", room, err), 500)
	}
	fmt.Printf("Successfully got channel: %s\n", room)

	// Get the actual WebSocket URL from Taubyte
	fmt.Printf("Getting WebSocket URL for channel: %s\n", room)
	wsURL, err := channel.WebSocket().Url()
	if err != nil {
		fmt.Printf("Error getting WebSocket URL for %s: %v\n", room, err)
		return fail(h, fmt.Errorf("failed to get WebSocket URL for %s: %v", room, err), 500)
	}
	fmt.Printf("Successfully got WebSocket URL for %s: %s\n", room, wsURL.Path)

	// Return the channel configuration with the actual WebSocket URL
	response := map[string]interface{}{
		"channel":       room,
		"room":          room,
		"protocol":      "taubyte-pubsub",
		"websocket_url": wsURL.Path,
	}

	responseData, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(responseData)
	h.Return(200)
	return 0
}

//export initCanvas
func initCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	if err := createCanvas(); err != nil {
		return fail(h, err, 500)
	}

	response := map[string]string{"message": "Canvas initialized successfully"}
	responseData, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(responseData)
	h.Return(200)
	return 0
}

//export resetCanvas
func resetCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Delete existing canvas
	db, err := getDB()
	if err != nil {
		return fail(h, err, 500)
	}

	if err := db.Delete("canvas"); err != nil {
		// Ignore error if canvas doesn't exist
	}

	// Clear cache
	cacheMutex.Lock()
	canvasCache = make([][]Pixel, 0)
	cacheValid = false
	cacheMutex.Unlock()

	// Initialize fresh canvas
	if err := createCanvas(); err != nil {
		return fail(h, err, 500)
	}

	response := map[string]string{"message": "Canvas reset successfully"}
	responseData, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(responseData)
	h.Return(200)
	return 0
}

// ===== Pub/Sub Handlers =====

//export onPixelUpdate
func onPixelUpdate(e event.Event) uint32 {
	channel, err := e.PubSub()
	if err != nil {
		fmt.Printf("Error getting PubSub channel in onPixelUpdate: %v\n", err)
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		fmt.Printf("Error getting channel data in onPixelUpdate: %v\n", err)
		return 1
	}

	var pixel Pixel
	if err := json.Unmarshal(data, &pixel); err != nil {
		fmt.Printf("Error unmarshaling pixel data: %v\n", err)
		return 1
	}

	// Validate pixel data
	if pixel.X < 0 || pixel.X >= CanvasWidth || pixel.Y < 0 || pixel.Y >= CanvasHeight {
		fmt.Printf("Invalid pixel coordinates: x=%d, y=%d (max: %dx%d)\n", pixel.X, pixel.Y, CanvasWidth, CanvasHeight)
		return 1
	}

	if !isValidColor(pixel.Color) {
		fmt.Printf("Invalid pixel color: %s\n", pixel.Color)
		return 1
	}

	// Save to database
	if err := savePixelToDB(pixel); err != nil {
		fmt.Printf("Error saving pixel to database: %v\n", err)
		return 1
	}

	return 0
}

//export onUserUpdate
func onUserUpdate(e event.Event) uint32 {
	channel, err := e.PubSub()
	if err != nil {
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		return 1
	}

	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return 1
	}

	// Validate user data
	if !isValidUsername(user.Username) {
		return 1
	}

	if !isValidColor(user.Color) {
		return 1
	}

	// Save to database
	if err := saveUserToDB(user); err != nil {
		return 1
	}

	return 0
}

//export onChatMessage
func onChatMessage(e event.Event) uint32 {
	channel, err := e.PubSub()
	if err != nil {
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		return 1
	}

	var message ChatMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return 1
	}

	// Validate and sanitize message
	if !isValidUsername(message.Username) {
		return 1
	}

	message.Message = sanitizeMessage(message.Message)
	if message.Message == "" {
		return 1
	}

	// Save to database
	if err := saveChatMessageToDB(message); err != nil {
		return 1
	}

	return 0
}
