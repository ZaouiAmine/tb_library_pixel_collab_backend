package lib

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	MaxMessages  = 50
	UserTimeout  = 30000 // 30 seconds in milliseconds
)

// ===== Data Structures =====

// Pixel represents a pixel on the canvas
type Pixel struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Color     string `json:"color"`
	UserID    string `json:"userId"`
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

// Response structures
type CanvasStateResponse struct {
	Canvas [][]Pixel `json:"canvas"`
	Width  int       `json:"width"`
	Height int       `json:"height"`
}

type UsersResponse struct {
	Users []User `json:"users"`
}

type ChatMessagesResponse struct {
	Messages []ChatMessage `json:"messages"`
}

// ===== Global State Management =====

// Database connections with connection pooling
var (
	canvasDB database.Database
	usersDB  database.Database
	chatDB   database.Database
	dbInit   sync.Once
)

// In-memory cache for better performance
var (
	canvasCache     [][]Pixel
	cacheMutex      sync.RWMutex
	lastCacheUpdate int64
	cacheValid      bool
)

// ===== Utility Functions =====

// fail writes error message and returns status code
func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 1
}

// generateMessageID creates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

// isValidPixel validates pixel coordinates
func isValidPixel(x, y int) bool {
	return x >= 0 && x < CanvasWidth && y >= 0 && y < CanvasHeight
}

// ===== Database Operations =====

// initDatabases initializes database connections
func initDatabases() {
	dbInit.Do(func() {
		var err error
		canvasDB, err = database.New("/canvas")
		if err != nil {
			fmt.Printf("Failed to initialize canvas database: %v\n", err)
		}

		usersDB, err = database.New("/users")
		if err != nil {
			fmt.Printf("Failed to initialize users database: %v\n", err)
		}

		chatDB, err = database.New("/chat")
		if err != nil {
			fmt.Printf("Failed to initialize chat database: %v\n", err)
		}

		fmt.Printf("Database connections initialized successfully\n")
	})
}

// getCanvasFromDB retrieves canvas from database with caching
func getCanvasFromDB() ([][]Pixel, error) {
	initDatabases()

	// Check cache first
	cacheMutex.RLock()
	if cacheValid && time.Now().UnixMilli()-lastCacheUpdate < 5000 { // 5 second cache
		canvas := make([][]Pixel, len(canvasCache))
		for i, row := range canvasCache {
			canvas[i] = make([]Pixel, len(row))
			copy(canvas[i], row)
		}
		cacheMutex.RUnlock()
		return canvas, nil
	}
	cacheMutex.RUnlock()

	// Initialize canvas if not exists
	widthData, err := canvasDB.Get("width")
	if err != nil {
		canvasDB.Put("width", []byte(strconv.Itoa(CanvasWidth)))
		canvasDB.Put("height", []byte(strconv.Itoa(CanvasHeight)))
		widthData = []byte(strconv.Itoa(CanvasWidth))
	}

	heightData, err := canvasDB.Get("height")
	if err != nil {
		canvasDB.Put("height", []byte(strconv.Itoa(CanvasHeight)))
		heightData = []byte(strconv.Itoa(CanvasHeight))
	}

	width, _ := strconv.Atoi(string(widthData))
	height, _ := strconv.Atoi(string(heightData))

	// Create empty canvas
	canvas := make([][]Pixel, height)
	for y := 0; y < height; y++ {
		canvas[y] = make([]Pixel, width)
		for x := 0; x < width; x++ {
			canvas[y][x] = Pixel{
				X: x, Y: y, Color: "#ffffff", UserID: "", Timestamp: 0,
			}
		}
	}

	// Load existing pixels
	keys, err := canvasDB.List("pixels/")
	if err == nil {
		for _, key := range keys {
			pixelData, err := canvasDB.Get(key)
			if err != nil {
				continue
			}

			var pixel Pixel
			if err := json.Unmarshal(pixelData, &pixel); err != nil {
				continue
			}

			if isValidPixel(pixel.X, pixel.Y) {
				canvas[pixel.Y][pixel.X] = pixel
			}
		}
	}

	// Update cache
	cacheMutex.Lock()
	canvasCache = canvas
	lastCacheUpdate = time.Now().UnixMilli()
	cacheValid = true
	cacheMutex.Unlock()

	return canvas, nil
}

// savePixelToDB saves pixel to database and updates cache
func savePixelToDB(pixel Pixel) error {
	initDatabases()

	pixelData, err := json.Marshal(pixel)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("pixels/%d_%d", pixel.X, pixel.Y)
	err = canvasDB.Put(key, pixelData)
	if err != nil {
		return err
	}

	// Update cache
	cacheMutex.Lock()
	if cacheValid && pixel.Y < len(canvasCache) && pixel.X < len(canvasCache[pixel.Y]) {
		canvasCache[pixel.Y][pixel.X] = pixel
	}
	cacheMutex.Unlock()

	return nil
}

// getUsersFromDB retrieves users from database
func getUsersFromDB() ([]User, error) {
	initDatabases()

	keys, err := usersDB.List("")
	if err != nil {
		return []User{}, nil
	}

	var users []User
	now := time.Now().UnixMilli()

	for _, key := range keys {
		userData, err := usersDB.Get(key)
		if err != nil {
			continue
		}

		var user User
		if err := json.Unmarshal(userData, &user); err != nil {
			continue
		}

		// Update online status
		user.IsOnline = (now - user.LastSeen) < UserTimeout
		users = append(users, user)
	}

	return users, nil
}

// saveUserToDB saves user to database
func saveUserToDB(user User) error {
	initDatabases()

	userData, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return usersDB.Put(user.ID, userData)
}

// getChatMessagesFromDB retrieves chat messages from database
func getChatMessagesFromDB() ([]ChatMessage, error) {
	initDatabases()

	keys, err := chatDB.List("")
	if err != nil {
		return []ChatMessage{}, nil
	}

	var messages []ChatMessage
	for _, key := range keys {
		messageData, err := chatDB.Get(key)
		if err != nil {
			continue
		}

		var message ChatMessage
		if err := json.Unmarshal(messageData, &message); err != nil {
			continue
		}

		messages = append(messages, message)
	}

	// Sort by timestamp and limit
	if len(messages) > MaxMessages {
		messages = messages[len(messages)-MaxMessages:]
	}

	return messages, nil
}

// saveChatMessageToDB saves chat message to database
func saveChatMessageToDB(message ChatMessage) error {
	initDatabases()

	messageData, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return chatDB.Put(message.ID, messageData)
}

// ===== Pub/Sub Functions =====

// publishPixelUpdate publishes pixel update
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

// publishUserUpdate publishes user update
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

// publishChatMessage publishes chat message
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

// placePixel places a pixel on the canvas
//
//export placePixel
func placePixel(e event.Event) uint32 {
	fmt.Printf("placePixel: Starting request\n")

	initDatabases()

	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("placePixel: Failed to get HTTP event: %v\n", err)
		return 1
	}

	// Get user ID from headers
	userID, err := h.Headers().Get("X-User-ID")
	if err != nil || userID == "" {
		return fail(h, fmt.Errorf("user ID required"), 401)
	}

	// Decode request body
	var req PlacePixelRequest
	dec := json.NewDecoder(h.Body())
	defer h.Body().Close()
	if err = dec.Decode(&req); err != nil {
		return fail(h, err, 400)
	}

	// Validate coordinates
	if !isValidPixel(req.X, req.Y) {
		return fail(h, fmt.Errorf("invalid coordinates"), 400)
	}

	// Get user from database
	userData, err := usersDB.Get(userID)
	if err != nil {
		return fail(h, fmt.Errorf("user not found"), 404)
	}

	var user User
	if err := json.Unmarshal(userData, &user); err != nil {
		return fail(h, err, 500)
	}

	// Create pixel
	pixel := Pixel{
		X:         req.X,
		Y:         req.Y,
		Color:     req.Color,
		UserID:    userID,
		Timestamp: time.Now().UnixMilli(),
	}

	// Save pixel to database
	if err := savePixelToDB(pixel); err != nil {
		return fail(h, err, 500)
	}

	// Update user stats
	user.PixelsPlaced++
	user.LastSeen = time.Now().UnixMilli()

	if err := saveUserToDB(user); err != nil {
		return fail(h, err, 500)
	}

	// Publish updates
	publishPixelUpdate(pixel)
	publishUserUpdate(user)

	h.Return(200)
	return 0
}

// joinGame joins a user to the game
//
//export joinGame
func joinGame(e event.Event) uint32 {
	fmt.Printf("joinGame: Starting request\n")

	initDatabases()

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Decode request body
	var req JoinGameRequest
	dec := json.NewDecoder(h.Body())
	defer h.Body().Close()
	if err = dec.Decode(&req); err != nil {
		return fail(h, err, 400)
	}

	// Validate input
	if req.Username == "" || req.UserID == "" {
		return fail(h, fmt.Errorf("username and user ID required"), 400)
	}

	// Create user
	user := User{
		ID:           req.UserID,
		Username:     req.Username,
		Color:        fmt.Sprintf("#%06x", time.Now().UnixNano()%16777216),
		IsOnline:     true,
		LastSeen:     time.Now().UnixMilli(),
		PixelsPlaced: 0,
	}

	// Save user to database
	if err := saveUserToDB(user); err != nil {
		return fail(h, err, 500)
	}

	// Publish user update
	publishUserUpdate(user)

	// Return user data
	userData, err := json.Marshal(user)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(userData)
	h.Return(200)
	return 0
}

// leaveGame removes a user from the game
//
//export leaveGame
func leaveGame(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get user ID from headers
	userID, err := h.Headers().Get("X-User-ID")
	if err != nil || userID == "" {
		return fail(h, fmt.Errorf("user ID required"), 401)
	}

	// Get user from database
	userData, err := usersDB.Get(userID)
	if err != nil {
		return fail(h, fmt.Errorf("user not found"), 404)
	}

	var user User
	if err := json.Unmarshal(userData, &user); err != nil {
		return fail(h, err, 500)
	}

	// Mark user as offline
	user.IsOnline = false
	user.LastSeen = time.Now().UnixMilli()

	// Save user to database
	if err := saveUserToDB(user); err != nil {
		return fail(h, err, 500)
	}

	// Publish user update
	publishUserUpdate(user)

	h.Return(200)
	return 0
}

// getCanvas returns the current canvas state
//
//export getCanvas
func getCanvas(e event.Event) uint32 {
	fmt.Printf("getCanvas: Starting request\n")

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get canvas from database
	canvas, err := getCanvasFromDB()
	if err != nil {
		return fail(h, err, 500)
	}

	// Create response
	response := CanvasStateResponse{
		Canvas: canvas,
		Width:  CanvasWidth,
		Height: CanvasHeight,
	}

	// Encode and send response
	jsonData, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(jsonData)
	h.Return(200)
	return 0
}

// getUsers returns the list of online users
//
//export getUsers
func getUsers(e event.Event) uint32 {
	fmt.Printf("getUsers: Starting request\n")

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get users from database
	users, err := getUsersFromDB()
	if err != nil {
		return fail(h, err, 500)
	}

	// Create response
	response := UsersResponse{Users: users}

	// Encode and send response
	jsonData, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(jsonData)
	h.Return(200)
	return 0
}

// sendMessage sends a chat message
//
//export sendMessage
func sendMessage(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get user ID from headers
	userID, err := h.Headers().Get("X-User-ID")
	if err != nil || userID == "" {
		return fail(h, fmt.Errorf("user ID required"), 401)
	}

	// Decode request body
	var req ChatMessageRequest
	dec := json.NewDecoder(h.Body())
	defer h.Body().Close()
	if err = dec.Decode(&req); err != nil {
		return fail(h, err, 400)
	}

	// Get user from database
	userData, err := usersDB.Get(userID)
	if err != nil {
		return fail(h, fmt.Errorf("user not found"), 404)
	}

	var user User
	if err := json.Unmarshal(userData, &user); err != nil {
		return fail(h, err, 500)
	}

	// Create chat message
	message := ChatMessage{
		ID:        generateMessageID(),
		UserID:    userID,
		Username:  user.Username,
		Message:   req.Message,
		Timestamp: time.Now().UnixMilli(),
		Type:      "user",
	}

	// Save message to database
	if err := saveChatMessageToDB(message); err != nil {
		return fail(h, err, 500)
	}

	// Publish chat message
	publishChatMessage(message)

	h.Return(200)
	return 0
}

// getMessages returns recent chat messages
//
//export getMessages
func getMessages(e event.Event) uint32 {
	fmt.Printf("getMessages: Starting request\n")

	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get messages from database
	messages, err := getChatMessagesFromDB()
	if err != nil {
		return fail(h, err, 500)
	}

	// Create response
	response := ChatMessagesResponse{Messages: messages}

	// Encode and send response
	jsonData, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(jsonData)
	h.Return(200)
	return 0
}

// ===== Pub/Sub Subscriber Functions =====

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

	// Update cache
	cacheMutex.Lock()
	if cacheValid && pixel.Y < len(canvasCache) && pixel.X < len(canvasCache[pixel.Y]) {
		canvasCache[pixel.Y][pixel.X] = pixel
	}
	cacheMutex.Unlock()

	fmt.Printf("Pixel processed: (%d,%d) by %s\n", pixel.X, pixel.Y, pixel.UserID)
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

	fmt.Printf("User update: id=%s, username=%s, online=%t\n",
		user.ID, user.Username, user.IsOnline)

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

	fmt.Printf("Chat message: %s: %s\n", message.Username, message.Message)
	return 0
}

// ===== WebSocket Functions =====

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
		"pixelcollab":  true,
	}

	if !allowedChannels[room] {
		return fail(h, fmt.Errorf("invalid room name: %s", room), 400)
	}

	// Get the pub/sub channel
	_, err = pubsub.Channel(room)
	if err != nil {
		return fail(h, fmt.Errorf("failed to get channel %s: %v", room, err), 500)
	}

	// Return the channel configuration
	response := map[string]interface{}{
		"channel":       room,
		"room":          room,
		"protocol":      "taubyte-pubsub",
		"websocket_url": fmt.Sprintf("ws/%s", room),
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

// initCanvas initializes canvas dimensions in database
//
//export initCanvas
func initCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	initDatabases()

	// Set canvas dimensions
	if err := canvasDB.Put("width", []byte(strconv.Itoa(CanvasWidth))); err != nil {
		return fail(h, err, 500)
	}
	if err := canvasDB.Put("height", []byte(strconv.Itoa(CanvasHeight))); err != nil {
		return fail(h, err, 500)
	}

	response := map[string]string{
		"status": "Canvas initialized",
		"width":  strconv.Itoa(CanvasWidth),
		"height": strconv.Itoa(CanvasHeight),
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
