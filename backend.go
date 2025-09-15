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
	CanvasWidth  = 32 // Match frontend canvas size
	CanvasHeight = 32
)

// ===== UTILITY FUNCTIONS =====
func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 1
}

// ===== HTTP HANDLERS =====

//export getCanvas
func getCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	db, err := database.New("/canvas")
	if err != nil {
		return fail(h, err, 500)
	}

	data, err := db.Get("data")
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
	h.Write(data)
	h.Return(200)
	return 0
}

//export placePixel
func placePixel(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get request body
	bodyBuffer := make([]byte, 1024) // Buffer for reading body
	bodyLen, err := h.Body().Read(bodyBuffer)
	if err != nil {
		return fail(h, err, 400)
	}

	var pixelUpdate struct {
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Color    string `json:"color"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
	}

	err = json.Unmarshal(bodyBuffer[:bodyLen], &pixelUpdate)
	if err != nil {
		return fail(h, err, 400)
	}

	// Validate pixel coordinates
	if pixelUpdate.X < 0 || pixelUpdate.X >= CanvasWidth ||
		pixelUpdate.Y < 0 || pixelUpdate.Y >= CanvasHeight {
		return fail(h, fmt.Errorf("invalid pixel coordinates"), 400)
	}

	// Load current canvas
	db, err := database.New("/canvas")
	if err != nil {
		return fail(h, err, 500)
	}

	canvasData, err := db.Get("data")
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
		db.Put("data", canvasData)
	}

	var canvas [][]string
	err = json.Unmarshal(canvasData, &canvas)
	if err != nil {
		return fail(h, err, 500)
	}

	// Update pixel
	canvas[pixelUpdate.Y][pixelUpdate.X] = pixelUpdate.Color

	// Save updated canvas
	updatedData, err := json.Marshal(canvas)
	if err != nil {
		return fail(h, err, 500)
	}
	err = db.Put("data", updatedData)
	if err != nil {
		return fail(h, err, 500)
	}

	// Broadcast the pixel update to all connected clients
	pixelChannel, err := pubsub.Channel("pixelupdates")
	if err == nil {
		pixelData, _ := json.Marshal(pixelUpdate)
		pixelChannel.Publish(pixelData)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write([]byte(`{"success": true}`))
	h.Return(200)
	return 0
}

//export initCanvas
func initCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	db, err := database.New("/canvas")
	if err != nil {
		return fail(h, err, 500)
	}

	// Create empty canvas
	emptyCanvas := make([][]string, CanvasHeight)
	for y := 0; y < CanvasHeight; y++ {
		emptyCanvas[y] = make([]string, CanvasWidth)
		for x := 0; x < CanvasWidth; x++ {
			emptyCanvas[y][x] = "#ffffff"
		}
	}

	jsonData, err := json.Marshal(emptyCanvas)
	if err != nil {
		return fail(h, err, 500)
	}

	err = db.Put("data", jsonData)
	if err != nil {
		return fail(h, err, 500)
	}

	// Broadcast canvas clear event
	clearChannel, err := pubsub.Channel("canvasupdates")
	if err == nil {
		clearData, _ := json.Marshal(map[string]string{"action": "clear"})
		clearChannel.Publish(clearData)
	}

	h.Write([]byte(`{"success": true}`))
	h.Return(200)
	return 0
}

//export getWebSocketURL
func getWebSocketURL(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Create/open a channel with hardcoded name
	channel, err := pubsub.Channel("pixelupdates")
	if err != nil {
		return fail(h, err, 500)
	}

	// Get the websocket url
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

//export getUsers
func getUsers(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	db, err := database.New("/users")
	if err != nil {
		return fail(h, err, 500)
	}

	data, err := db.Get("data")
	if err != nil {
		// Return empty users array if no data exists
		jsonData, _ := json.Marshal([]User{})
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

//export joinGame
func joinGame(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get request body
	bodyBuffer := make([]byte, 1024) // Buffer for reading body
	bodyLen, err := h.Body().Read(bodyBuffer)
	if err != nil {
		return fail(h, err, 400)
	}

	var user User
	err = json.Unmarshal(bodyBuffer[:bodyLen], &user)
	if err != nil {
		return fail(h, err, 400)
	}

	// Set user as online
	user.Online = true

	// Load current users
	db, err := database.New("/users")
	if err != nil {
		return fail(h, err, 500)
	}

	usersData, err := db.Get("data")
	if err != nil {
		// If no data exists, create empty array
		usersData = []byte("[]")
	}

	var users []User
	err = json.Unmarshal(usersData, &users)
	if err != nil {
		users = []User{}
	}

	// Update users list
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

	// Save updated users
	updatedData, err := json.Marshal(users)
	if err != nil {
		return fail(h, err, 500)
	}
	err = db.Put("data", updatedData)
	if err != nil {
		return fail(h, err, 500)
	}

	// Broadcast the user join to all connected clients
	userChannel, err := pubsub.Channel("userupdates")
	if err == nil {
		userData, _ := json.Marshal(user)
		userChannel.Publish(userData)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write([]byte(`{"success": true}`))
	h.Return(200)
	return 0
}

//export leaveGame
func leaveGame(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get request body
	bodyBuffer := make([]byte, 1024) // Buffer for reading body
	bodyLen, err := h.Body().Read(bodyBuffer)
	if err != nil {
		return fail(h, err, 400)
	}

	var userUpdate struct {
		UserID string `json:"userId"`
	}

	err = json.Unmarshal(bodyBuffer[:bodyLen], &userUpdate)
	if err != nil {
		return fail(h, err, 400)
	}

	// Load current users
	db, err := database.New("/users")
	if err != nil {
		return fail(h, err, 500)
	}

	usersData, err := db.Get("data")
	if err != nil {
		// If no data exists, return success
		h.Write([]byte(`{"success": true}`))
		h.Return(200)
		return 0
	}

	var users []User
	err = json.Unmarshal(usersData, &users)
	if err != nil {
		users = []User{}
	}

	// Update user as offline
	found := false
	for i, u := range users {
		if u.ID == userUpdate.UserID {
			users[i].Online = false
			found = true
			break
		}
	}

	if found {
		// Save updated users
		updatedData, err := json.Marshal(users)
		if err != nil {
			return fail(h, err, 500)
		}
		err = db.Put("data", updatedData)
		if err != nil {
			return fail(h, err, 500)
		}

		// Broadcast the user leave to all connected clients
		userChannel, err := pubsub.Channel("userupdates")
		if err == nil {
			// Find the user and broadcast their offline status
			for _, u := range users {
				if u.ID == userUpdate.UserID {
					userData, _ := json.Marshal(u)
					userChannel.Publish(userData)
					break
				}
			}
		}
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write([]byte(`{"success": true}`))
	h.Return(200)
	return 0
}

//export getMessages
func getMessages(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	db, err := database.New("/chat")
	if err != nil {
		return fail(h, err, 500)
	}

	data, err := db.Get("data")
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

//export sendMessage
func sendMessage(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// Get request body
	bodyBuffer := make([]byte, 1024) // Buffer for reading body
	bodyLen, err := h.Body().Read(bodyBuffer)
	if err != nil {
		return fail(h, err, 400)
	}

	var message ChatMessage
	err = json.Unmarshal(bodyBuffer[:bodyLen], &message)
	if err != nil {
		return fail(h, err, 400)
	}

	// Load current messages
	db, err := database.New("/chat")
	if err != nil {
		return fail(h, err, 500)
	}

	messagesData, err := db.Get("data")
	if err != nil {
		// If no data exists, create empty array
		messagesData = []byte("[]")
	}

	var messages []ChatMessage
	err = json.Unmarshal(messagesData, &messages)
	if err != nil {
		messages = []ChatMessage{}
	}

	// Add message
	message.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	message.Timestamp = time.Now().Unix()
	messages = append(messages, message)

	// Keep only last 100 messages
	if len(messages) > 100 {
		messages = messages[len(messages)-100:]
	}

	// Save updated messages
	updatedData, err := json.Marshal(messages)
	if err != nil {
		return fail(h, err, 500)
	}
	err = db.Put("data", updatedData)
	if err != nil {
		return fail(h, err, 500)
	}

	// Broadcast the chat message to all connected clients
	chatChannel, err := pubsub.Channel("chatmessages")
	if err == nil {
		messageData, _ := json.Marshal(message)
		chatChannel.Publish(messageData)
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Write([]byte(`{"success": true}`))
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

	var pixelUpdate struct {
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Color    string `json:"color"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
	}

	err = json.Unmarshal(data, &pixelUpdate)
	if err != nil {
		return 1
	}

	// Update canvas in database
	db, err := database.New("/canvas")
	if err != nil {
		return 1
	}

	canvasData, err := db.Get("data")
	if err != nil {
		return 1
	}

	var canvas [][]string
	err = json.Unmarshal(canvasData, &canvas)
	if err != nil {
		return 1
	}

	// Update pixel
	if pixelUpdate.X >= 0 && pixelUpdate.X < CanvasWidth &&
		pixelUpdate.Y >= 0 && pixelUpdate.Y < CanvasHeight {
		canvas[pixelUpdate.Y][pixelUpdate.X] = pixelUpdate.Color

		// Save updated canvas
		updatedData, err := json.Marshal(canvas)
		if err != nil {
			return 1
		}
		err = db.Put("data", updatedData)
		if err != nil {
			return 1
		}
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
	err = json.Unmarshal(data, &user)
	if err != nil {
		return 1
	}

	// Update users in database
	db, err := database.New("/users")
	if err != nil {
		return 1
	}

	usersData, err := db.Get("data")
	if err != nil {
		usersData = []byte("[]")
	}

	var users []User
	err = json.Unmarshal(usersData, &users)
	if err != nil {
		users = []User{}
	}

	// Update users list
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

	// Save updated users
	updatedData, err := json.Marshal(users)
	if err != nil {
		return 1
	}
	err = db.Put("data", updatedData)
	if err != nil {
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
	err = json.Unmarshal(data, &message)
	if err != nil {
		return 1
	}

	// Update messages in database
	db, err := database.New("/chat")
	if err != nil {
		return 1
	}

	messagesData, err := db.Get("data")
	if err != nil {
		messagesData = []byte("[]")
	}

	var messages []ChatMessage
	err = json.Unmarshal(messagesData, &messages)
	if err != nil {
		messages = []ChatMessage{}
	}

	// Add message
	messages = append(messages, message)

	// Keep only last 100 messages
	if len(messages) > 100 {
		messages = messages[len(messages)-100:]
	}

	// Save updated messages
	updatedData, err := json.Marshal(messages)
	if err != nil {
		return 1
	}
	err = db.Put("data", updatedData)
	if err != nil {
		return 1
	}

	return 0
}
