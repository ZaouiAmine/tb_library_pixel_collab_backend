package lib

import (
	"encoding/json"
	"fmt"
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
	UserTimeout  = 30000 // 30 seconds in milliseconds
)

// ===== Data Structures =====

// Pixel represents a pixel on the canvas
type Pixel struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Color     string `json:"color"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Timestamp int64  `json:"timestamp"`
}

// User represents a user in the game
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Color        string `json:"color"`
	IsOnline     bool   `json:"isOnline"`
	LastSeen     int64  `json:"lastSeen"`
	PixelsPlaced int    `json:"pixelsPlaced"`
}

// ChatMessage represents a chat message
type ChatMessage struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	Type      string `json:"type"`
}

// Request structures
type PlacePixelRequest struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

type JoinGameRequest struct {
	Username string `json:"username"`
	UserID   string `json:"userId"`
}

type ChatMessageRequest struct {
	Message string `json:"message"`
}

// ===== Global State =====
var (
	canvasCache [][]Pixel
	cacheMutex  sync.RWMutex
	cacheValid  bool
)

// ===== Utility Functions =====

func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 1
}

func isValidPixel(x, y int) bool {
	return x >= 0 && x < CanvasWidth && y >= 0 && y < CanvasHeight
}

func generateMessageID() string {
	return fmt.Sprintf("msg_%d_%d", time.Now().UnixMilli(), time.Now().UnixNano()%1000000)
}

// ===== Database Operations =====

func initCanvas() error {
	db, err := database.New("/canvas")
	if err != nil {
		return err
	}

	// Initialize empty canvas
	canvas := make([][]Pixel, CanvasHeight)
	for y := range canvas {
		canvas[y] = make([]Pixel, CanvasWidth)
	}

	canvasData, err := json.Marshal(canvas)
	if err != nil {
		return err
	}

	return db.Put("canvas", canvasData)
}

func getCanvasFromDB() ([][]Pixel, error) {
	db, err := database.New("/canvas")
	if err != nil {
		return nil, err
	}

	data, err := db.Get("canvas")
	if err != nil {
		// Initialize canvas if it doesn't exist
		if err := initCanvas(); err != nil {
			return nil, err
		}
		data, err = db.Get("canvas")
		if err != nil {
			return nil, err
		}
	}

	var canvas [][]Pixel
	if err := json.Unmarshal(data, &canvas); err != nil {
		return nil, err
	}

	return canvas, nil
}

func savePixelToDB(pixel Pixel) error {
	db, err := database.New("/canvas")
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

	return db.Put("canvas", canvasData)
}

func getUsersFromDB() ([]User, error) {
	db, err := database.New("/users")
	if err != nil {
		return nil, err
	}

	keys, err := db.List("")
	if err != nil {
		return []User{}, nil
	}

	var users []User
	now := time.Now().UnixMilli()

	for _, key := range keys {
		userData, err := db.Get(key)
		if err != nil {
			continue
		}

		var user User
		if err := json.Unmarshal(userData, &user); err != nil {
			continue
		}

		// Filter out offline users
		if (now - user.LastSeen) <= UserTimeout {
			users = append(users, user)
		}
	}

	return users, nil
}

func saveUserToDB(user User) error {
	db, err := database.New("/users")
	if err != nil {
		return err
	}

	userData, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return db.Put(user.ID, userData)
}

func getChatMessagesFromDB() ([]ChatMessage, error) {
	db, err := database.New("/chat")
	if err != nil {
		return nil, err
	}

	keys, err := db.List("")
	if err != nil {
		return []ChatMessage{}, nil
	}

	var messages []ChatMessage
	for _, key := range keys {
		messageData, err := db.Get(key)
		if err != nil {
			continue
		}

		var message ChatMessage
		if err := json.Unmarshal(messageData, &message); err != nil {
			continue
		}

		messages = append(messages, message)
	}

	// Sort by timestamp (newest first) and limit
	if len(messages) > MaxMessages {
		messages = messages[:MaxMessages]
	}

	return messages, nil
}

func saveChatMessageToDB(message ChatMessage) error {
	db, err := database.New("/chat")
	if err != nil {
		return err
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return db.Put(message.ID, messageData)
}

// ===== Pub/Sub Operations =====

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

// getCanvas returns the current canvas state
//
//export getCanvas
func getCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
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

	// Fallback to database
	canvas, err := getCanvasFromDB()
	if err != nil {
		return fail(h, err, 500)
	}

	canvasData, err := json.Marshal(canvas)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(canvasData)
	h.Return(200)
	return 0
}

// getUsers returns online users
//
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

// getMessages returns recent chat messages
//
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

// getWebSocketURL returns WebSocket configuration for Taubyte
//
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
	channel, err := pubsub.Channel(room)
	if err != nil {
		return fail(h, fmt.Errorf("failed to get channel %s: %v", room, err), 500)
	}

	// Get the actual WebSocket URL from Taubyte
	wsURL, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, fmt.Errorf("failed to get WebSocket URL for %s: %v", room, err), 500)
	}

	// Return the channel configuration with the actual WebSocket URL
	response := map[string]interface{}{
		"channel":       room,
		"room":          room,
		"protocol":      "taubyte-pubsub",
		"websocket_url": wsURL.Path,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(jsonData)
	h.Return(200)
	return 0
}

// initCanvasHandler initializes canvas dimensions in database
//
//export initCanvas
func initCanvasHandler(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	if err := initCanvas(); err != nil {
		return fail(h, err, 500)
	}

	h.Return(200)
	return 0
}

// ===== Pub/Sub Subscribers (Background Processing) =====

// onPixelUpdate handles incoming pixel updates from pub/sub
//
//export onPixelUpdate
func onPixelUpdate(e event.Event) uint32 {
	ps, err := e.PubSub()
	if err != nil {
		fmt.Printf("Error getting pubsub event: %v\n", err)
		return 1
	}

	data, err := ps.Data()
	if err != nil {
		fmt.Printf("Error getting pixel update data: %v\n", err)
		return 1
	}

	var pixel Pixel
	if err := json.Unmarshal(data, &pixel); err != nil {
		fmt.Printf("Error unmarshaling pixel update: %v\n", err)
		return 1
	}

	// Validate pixel coordinates
	if !isValidPixel(pixel.X, pixel.Y) {
		fmt.Printf("Invalid pixel coordinates: (%d,%d)\n", pixel.X, pixel.Y)
		return 1
	}

	// Validate user exists and is online
	db, err := database.New("/users")
	if err != nil {
		fmt.Printf("Error creating users database: %v\n", err)
		return 1
	}

	userData, err := db.Get(pixel.UserID)
	if err != nil {
		fmt.Printf("User not found: %s\n", pixel.UserID)
		return 1
	}

	var user User
	if err := json.Unmarshal(userData, &user); err != nil {
		fmt.Printf("Error unmarshaling user: %v\n", err)
		return 1
	}

	// Check if user is online
	now := time.Now().UnixMilli()
	if (now - user.LastSeen) > UserTimeout {
		fmt.Printf("User %s is offline\n", pixel.UserID)
		return 1
	}

	// Pixel limit removed - users can place unlimited pixels

	// Add username to pixel
	pixel.Username = user.Username

	// Save pixel to database
	if err := savePixelToDB(pixel); err != nil {
		fmt.Printf("Error saving pixel to database: %v\n", err)
		return 1
	}

	// Update user stats
	user.PixelsPlaced++
	user.LastSeen = now
	if err := saveUserToDB(user); err != nil {
		fmt.Printf("Error updating user stats: %v\n", err)
		// Don't fail completely, pixel was saved
	}

	// Update cache
	cacheMutex.Lock()
	if !cacheValid {
		canvas, err := getCanvasFromDB()
		if err == nil {
			canvasCache = canvas
			cacheValid = true
		}
	}
	if cacheValid && pixel.Y < len(canvasCache) && pixel.X < len(canvasCache[pixel.Y]) {
		canvasCache[pixel.Y][pixel.X] = pixel
	}
	cacheMutex.Unlock()

	fmt.Printf("Pixel validated and saved: (%d,%d) by %s (total pixels: %d)\n",
		pixel.X, pixel.Y, pixel.UserID, user.PixelsPlaced)
	return 0
}

// onUserUpdate handles incoming user updates from pub/sub
//
//export onUserUpdate
func onUserUpdate(e event.Event) uint32 {
	ps, err := e.PubSub()
	if err != nil {
		fmt.Printf("Error getting pubsub event: %v\n", err)
		return 1
	}

	data, err := ps.Data()
	if err != nil {
		fmt.Printf("Error getting user update data: %v\n", err)
		return 1
	}

	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		fmt.Printf("Error unmarshaling user update: %v\n", err)
		return 1
	}

	// Validate user data
	if user.ID == "" || user.Username == "" {
		fmt.Printf("Invalid user data: missing ID or username\n")
		return 1
	}

	// Save user to database
	if err := saveUserToDB(user); err != nil {
		fmt.Printf("Error saving user to database: %v\n", err)
		return 1
	}

	fmt.Printf("User validated and saved: id=%s, username=%s, online=%t, pixels=%d\n",
		user.ID, user.Username, user.IsOnline, user.PixelsPlaced)

	return 0
}

// onChatMessage handles incoming chat messages from pub/sub
//
//export onChatMessage
func onChatMessage(e event.Event) uint32 {
	ps, err := e.PubSub()
	if err != nil {
		fmt.Printf("Error getting pubsub event: %v\n", err)
		return 1
	}

	data, err := ps.Data()
	if err != nil {
		fmt.Printf("Error getting chat message data: %v\n", err)
		return 1
	}

	var message ChatMessage
	if err := json.Unmarshal(data, &message); err != nil {
		fmt.Printf("Error unmarshaling chat message: %v\n", err)
		return 1
	}

	// Validate message data
	if message.UserID == "" || message.Username == "" || message.Message == "" {
		fmt.Printf("Invalid chat message: missing required fields\n")
		return 1
	}

	// Validate user exists
	db, err := database.New("/users")
	if err != nil {
		fmt.Printf("Error creating users database: %v\n", err)
		return 1
	}

	_, err = db.Get(message.UserID)
	if err != nil {
		fmt.Printf("User not found for chat message: %s\n", message.UserID)
		return 1
	}

	// Save message to database
	if err := saveChatMessageToDB(message); err != nil {
		fmt.Printf("Error saving chat message to database: %v\n", err)
		return 1
	}

	fmt.Printf("Chat message validated and saved: %s: %s\n", message.Username, message.Message)
	return 0
}
