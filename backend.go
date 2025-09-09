package lib

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/taubyte/go-sdk/database"
	"github.com/taubyte/go-sdk/event"
	http "github.com/taubyte/go-sdk/http/event"
	pubsub "github.com/taubyte/go-sdk/pubsub/node"
)

// Shared helper for errors → writes error message + returns status code
//

func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 1
}

// ===== Data Structures =====

// Represents a pixel on the canvas
type Pixel struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Color     string `json:"color"`
	UserID    string `json:"userId"`
	Timestamp int64  `json:"timestamp"`
}

// Represents a user in the game
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Color        string `json:"color"`
	IsOnline     bool   `json:"isOnline"`
	LastSeen     int64  `json:"lastSeen"`
	PixelsPlaced int    `json:"pixelsPlaced"`
}

// Represents a chat message
type ChatMessage struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	Type      string `json:"type"`
}

// Request to place a pixel
type PlacePixelRequest struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

// Request to join the game
type JoinGameRequest struct {
	Username string `json:"username"`
	UserID   string `json:"userId"`
}

// Request to send a chat message
type ChatMessageRequest struct {
	Message string `json:"message"`
}

// Canvas state response
type CanvasStateResponse struct {
	Canvas [][]Pixel `json:"canvas"`
	Width  int       `json:"width"`
	Height int       `json:"height"`
}

// Users list response
type UsersResponse struct {
	Users []User `json:"users"`
}

// Chat messages response
type ChatMessagesResponse struct {
	Messages []ChatMessage `json:"messages"`
}

// ===== Utility Functions =====

// Generate a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

// Validate pixel coordinates
func isValidPixel(x, y, width, height int) bool {
	return x >= 0 && x < width && y >= 0 && y < height
}

// Cooldown functions removed - no longer needed

// ===== Database Operations =====

// Get canvas from database
func getCanvasFromDB() ([][]Pixel, error) {
	db, err := database.New("/canvas")
	if err != nil {
		return nil, err
	}

	// Get canvas dimensions
	widthData, err := db.Get("width")
	var heightData []byte
	if err != nil {
		// Canvas not initialized, initialize it with default dimensions
		if err := db.Put("width", []byte("100")); err != nil {
			return nil, err
		}
		if err := db.Put("height", []byte("100")); err != nil {
			return nil, err
		}
		widthData = []byte("100")
		heightData = []byte("100")
	} else {
		heightData, err = db.Get("height")
		if err != nil {
			// Height not found, set default
			if err := db.Put("height", []byte("100")); err != nil {
				return nil, err
			}
			heightData = []byte("100")
		}
	}

	width, _ := strconv.Atoi(string(widthData))
	height, _ := strconv.Atoi(string(heightData))

	// Initialize empty canvas
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
	keys, err := db.List("pixels/")
	if err == nil {
		for _, key := range keys {
			pixelData, err := db.Get(key)
			if err != nil {
				continue
			}

			var pixel Pixel
			if err := json.Unmarshal(pixelData, &pixel); err != nil {
				continue
			}

			if isValidPixel(pixel.X, pixel.Y, width, height) {
				canvas[pixel.Y][pixel.X] = pixel
			}
		}
	}

	return canvas, nil
}

// Save pixel to database
func savePixelToDB(pixel Pixel) error {
	fmt.Printf("savePixelToDB: Starting to save pixel at (%d,%d)\n", pixel.X, pixel.Y)

	db, err := database.New("/canvas")
	if err != nil {
		fmt.Printf("savePixelToDB: Failed to connect to canvas database: %v\n", err)
		return err
	}

	pixelData, err := json.Marshal(pixel)
	if err != nil {
		fmt.Printf("savePixelToDB: Failed to marshal pixel data: %v\n", err)
		return err
	}

	key := fmt.Sprintf("pixels/%d_%d", pixel.X, pixel.Y)
	fmt.Printf("savePixelToDB: Saving pixel with key: %s\n", key)

	err = db.Put(key, pixelData)
	if err != nil {
		fmt.Printf("savePixelToDB: Failed to save pixel to database: %v\n", err)
		return err
	}

	fmt.Printf("savePixelToDB: Pixel saved successfully\n")
	return nil
}

// Get users from database
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
	for _, key := range keys {
		userData, err := db.Get(key)
		if err != nil {
			continue
		}

		var user User
		if err := json.Unmarshal(userData, &user); err != nil {
			continue
		}

		// Check if user is still online (within last 30 seconds)
		if time.Now().UnixMilli()-user.LastSeen < 30000 {
			user.IsOnline = true
		} else {
			user.IsOnline = false
		}

		users = append(users, user)
	}

	return users, nil
}

// Save user to database
func saveUserToDB(user User) error {
	fmt.Printf("saveUserToDB: Starting to save user: %s\n", user.ID)

	db, err := database.New("/users")
	if err != nil {
		fmt.Printf("saveUserToDB: Failed to connect to users database: %v\n", err)
		return err
	}

	userData, err := json.Marshal(user)
	if err != nil {
		fmt.Printf("saveUserToDB: Failed to marshal user data: %v\n", err)
		return err
	}

	err = db.Put(user.ID, userData)
	if err != nil {
		fmt.Printf("saveUserToDB: Failed to save user to database: %v\n", err)
		return err
	}

	fmt.Printf("saveUserToDB: User saved successfully\n")
	return nil
}

// Get chat messages from database
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

	return messages, nil
}

// Save chat message to database
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

// ===== Pub/Sub Functions =====

// Publish pixel update
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

// Publish user update
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

// Publish chat message
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

// ===== Exported Functions (HTTP Handlers) =====

// placePixel → Places a pixel on the canvas
// Path: /api/placePixel
//
//export placePixel
func placePixel(e event.Event) uint32 {
	fmt.Printf("placePixel: Starting pixel placement request\n")

	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("placePixel: Failed to get HTTP event: %v\n", err)
		return 1
	}

	// Get user ID from headers
	userID, err := h.Headers().Get("X-User-ID")
	if err != nil || userID == "" {
		fmt.Printf("placePixel: User ID missing or invalid: %v\n", err)
		return fail(h, fmt.Errorf("user ID required"), 401)
	}
	fmt.Printf("placePixel: User ID: %s\n", userID)

	// Decode request body
	var req PlacePixelRequest
	dec := json.NewDecoder(h.Body())
	defer h.Body().Close()
	if err = dec.Decode(&req); err != nil {
		fmt.Printf("placePixel: Failed to decode request body: %v\n", err)
		return fail(h, err, 400)
	}
	fmt.Printf("placePixel: Request - X:%d, Y:%d, Color:%s\n", req.X, req.Y, req.Color)

	// Get user from database
	db, err := database.New("/users")
	if err != nil {
		fmt.Printf("placePixel: Failed to connect to users database: %v\n", err)
		return fail(h, err, 500)
	}

	userData, err := db.Get(userID)
	if err != nil {
		fmt.Printf("placePixel: User not found in database: %s, error: %v\n", userID, err)
		return fail(h, fmt.Errorf("user not found"), 404)
	}

	var user User
	if err := json.Unmarshal(userData, &user); err != nil {
		fmt.Printf("placePixel: Failed to unmarshal user data: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Printf("placePixel: User found - ID:%s, Username:%s, PixelsPlaced:%d\n", user.ID, user.Username, user.PixelsPlaced)

	// Cooldown removed - allow immediate pixel placement

	// Validate pixel coordinates (assuming 100x100 canvas)
	if !isValidPixel(req.X, req.Y, 100, 100) {
		fmt.Printf("placePixel: Invalid coordinates - X:%d, Y:%d\n", req.X, req.Y)
		return fail(h, fmt.Errorf("invalid coordinates"), 400)
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
		fmt.Printf("placePixel: Failed to save pixel to database: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Printf("placePixel: Pixel saved successfully\n")

	// Update user stats
	user.PixelsPlaced++
	user.LastSeen = time.Now().UnixMilli()

	if err := saveUserToDB(user); err != nil {
		fmt.Printf("placePixel: Failed to save user to database: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Printf("placePixel: User stats updated successfully\n")

	// Publish pixel update
	if err := publishPixelUpdate(pixel); err != nil {
		// Log error but don't fail the request
		fmt.Printf("placePixel: Failed to publish pixel update: %v\n", err)
	} else {
		fmt.Printf("placePixel: Pixel update published successfully\n")
	}

	// Publish user update
	if err := publishUserUpdate(user); err != nil {
		fmt.Printf("placePixel: Failed to publish user update: %v\n", err)
	} else {
		fmt.Printf("placePixel: User update published successfully\n")
	}

	fmt.Printf("placePixel: Request completed successfully\n")
	h.Return(200)
	return 0
}

// joinGame → Joins a user to the game
// Path: /api/joinGame
//
//export joinGame
func joinGame(e event.Event) uint32 {
	fmt.Printf("joinGame: Starting join game request\n")

	h, err := e.HTTP()
	if err != nil {
		fmt.Printf("joinGame: Failed to get HTTP event: %v\n", err)
		return 1
	}

	// Decode request body
	var req JoinGameRequest
	dec := json.NewDecoder(h.Body())
	defer h.Body().Close()
	if err = dec.Decode(&req); err != nil {
		fmt.Printf("joinGame: Failed to decode request body: %v\n", err)
		return fail(h, err, 400)
	}
	fmt.Printf("joinGame: Request - Username:%s, UserID:%s\n", req.Username, req.UserID)

	// Validate input
	if req.Username == "" || req.UserID == "" {
		fmt.Printf("joinGame: Invalid input - Username:%s, UserID:%s\n", req.Username, req.UserID)
		return fail(h, fmt.Errorf("username and user ID required"), 400)
	}

	// Create user
	user := User{
		ID:           req.UserID,
		Username:     req.Username,
		Color:        fmt.Sprintf("#%06x", time.Now().UnixNano()%16777216), // Random color
		IsOnline:     true,
		LastSeen:     time.Now().UnixMilli(),
		PixelsPlaced: 0,
	}
	fmt.Printf("joinGame: Created user - ID:%s, Username:%s, Color:%s\n", user.ID, user.Username, user.Color)

	// Save user to database
	if err := saveUserToDB(user); err != nil {
		fmt.Printf("joinGame: Failed to save user to database: %v\n", err)
		return fail(h, err, 500)
	}
	fmt.Printf("joinGame: User saved to database successfully\n")

	// Publish user update
	if err := publishUserUpdate(user); err != nil {
		fmt.Printf("joinGame: Failed to publish user update: %v\n", err)
	} else {
		fmt.Printf("joinGame: User update published successfully\n")
	}

	// Return user data
	userData, err := json.Marshal(user)
	if err != nil {
		fmt.Printf("joinGame: Failed to marshal user data: %v\n", err)
		return fail(h, err, 500)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write(userData)
	fmt.Printf("joinGame: Request completed successfully\n")
	h.Return(200)
	return 0
}

// leaveGame → Removes a user from the game
// Path: /api/leaveGame
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
	db, err := database.New("/users")
	if err != nil {
		return fail(h, err, 500)
	}

	userData, err := db.Get(userID)
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
	if err := publishUserUpdate(user); err != nil {
		fmt.Printf("Failed to publish user update: %v\n", err)
	}

	h.Return(200)
	return 0
}

// getCanvas → Returns the current canvas state
// Path: /api/getCanvas
//
//export getCanvas
func getCanvas(e event.Event) uint32 {
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
		Width:  100,
		Height: 100,
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

// getUsers → Returns the list of online users
// Path: /api/getUsers
//
//export getUsers
func getUsers(e event.Event) uint32 {
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

// sendMessage → Sends a chat message
// Path: /api/sendMessage
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
	db, err := database.New("/users")
	if err != nil {
		return fail(h, err, 500)
	}

	userData, err := db.Get(userID)
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
	if err := publishChatMessage(message); err != nil {
		fmt.Printf("Failed to publish chat message: %v\n", err)
	}

	h.Return(200)
	return 0
}

// getMessages → Returns recent chat messages
// Path: /api/getMessages
//
//export getMessages
func getMessages(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get messages from database
	messages, err := getChatMessagesFromDB()
	if err != nil {
		return fail(h, err, 500)
	}

	// Limit to last 50 messages
	if len(messages) > 50 {
		messages = messages[len(messages)-50:]
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

// onPixelUpdate → Handles incoming pixel updates from pub/sub
//
//export onPixelUpdate
func onPixelUpdate(e event.Event) uint32 {
	// Get the pubsub event
	ps, err := e.PubSub()
	if err != nil {
		fmt.Printf("Error getting pubsub event: %v\n", err)
		return 1
	}

	// Get the pixel data from the pub/sub message
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

	// Save pixel to database
	if err := savePixelToDB(pixel); err != nil {
		fmt.Printf("Error saving pixel: %v\n", err)
		return 1
	}

	// Update user stats
	db, err := database.New("/users")
	if err == nil {
		userData, err := db.Get(pixel.UserID)
		if err == nil {
			var user User
			if err := json.Unmarshal(userData, &user); err == nil {
				user.PixelsPlaced++
				user.LastSeen = time.Now().UnixMilli()
				saveUserToDB(user)
			}
		}
	}

	fmt.Printf("Pixel saved: (%d,%d) by %s\n", pixel.X, pixel.Y, pixel.UserID)

	return 0
}

// onUserUpdate → Handles incoming user updates from pub/sub
//
//export onUserUpdate
func onUserUpdate(e event.Event) uint32 {
	// Get the pubsub event
	ps, err := e.PubSub()
	if err != nil {
		fmt.Printf("Error getting pubsub event: %v\n", err)
		return 1
	}

	// Get the user data from the pub/sub message
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

	// Broadcast user update to WebSocket clients
	updateMsg := map[string]interface{}{
		"type":      "userUpdate",
		"payload":   user,
		"timestamp": time.Now().UnixMilli(),
	}

	_, err = json.Marshal(updateMsg)
	if err != nil {
		fmt.Printf("Error marshaling user update: %v\n", err)
	} else {
		// In a real Taubyte implementation, this would broadcast to WebSocket clients
		fmt.Printf("User update broadcast: id=%s, username=%s, online=%t\n",
			user.ID, user.Username, user.IsOnline)
	}

	return 0
}

// onChatMessage → Handles incoming chat messages from pub/sub
//
//export onChatMessage
func onChatMessage(e event.Event) uint32 {
	// Get the pubsub event
	ps, err := e.PubSub()
	if err != nil {
		fmt.Printf("Error getting pubsub event: %v\n", err)
		return 1
	}

	// Get the message data from the pub/sub message
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

	// Save message to database
	if err := saveChatMessageToDB(message); err != nil {
		fmt.Printf("Error saving chat message: %v\n", err)
		return 1
	}

	fmt.Printf("Chat message saved: %s: %s\n", message.Username, message.Message)

	return 0
}

// ===== WebSocket Relay Functions =====

// WebSocket functionality is handled by Taubyte's built-in WebSocket support
// The pub/sub system will automatically broadcast messages to connected WebSocket clients

// getWebSocketURL → Returns WebSocket configuration for Taubyte
// Path: /api/getWebSocketURL
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
		room = "pixelupdates" // Default room name
	}

	// Validate that the room name is one of our allowed channels
	allowedChannels := map[string]bool{
		"pixelupdates": true,
		"chatmessages": true,
		"userupdates":  true,
		"pixelcollab":  true,
	}

	if !allowedChannels[room] {
		return fail(h, fmt.Errorf("invalid room name: %s", room), 400)
	}

	// Get the pub/sub channel for real-time communication
	_, err = pubsub.Channel(room)
	if err != nil {
		return fail(h, fmt.Errorf("failed to get channel %s: %v", room, err), 500)
	}

	// Return the channel name for frontend to use
	response := map[string]interface{}{
		"channel":       room,
		"room":          room,
		"protocol":      "taubyte-pubsub",
		"websocket_url": fmt.Sprintf("ws/%s", room), // Taubyte WebSocket URL format
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

// initCanvas → Initializes canvas dimensions in database
// Path: /api/initCanvas
//
//export initCanvas
func initCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Initialize canvas database with default dimensions
	db, err := database.New("/canvas")
	if err != nil {
		return fail(h, err, 500)
	}

	// Set canvas dimensions
	if err := db.Put("width", []byte("100")); err != nil {
		return fail(h, err, 500)
	}
	if err := db.Put("height", []byte("100")); err != nil {
		return fail(h, err, 500)
	}

	response := map[string]string{
		"status": "Canvas initialized",
		"width":  "100",
		"height": "100",
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
