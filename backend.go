package lib

import (
	"crypto/md5"
	"encoding/hex"
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
	CanvasWidth  = 100
	CanvasHeight = 100
)

// ===== UTILITY FUNCTIONS =====
func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 1
}

func getCanvasDB() (database.Database, error) {
	return database.New("/canvas")
}

func getUsersDB() (database.Database, error) {
	return database.New("/users")
}

func getChatDB() (database.Database, error) {
	return database.New("/chat")
}

func createEmptyCanvas() [][]Pixel {
	canvas := make([][]Pixel, CanvasHeight)
	for y := 0; y < CanvasHeight; y++ {
		canvas[y] = make([]Pixel, CanvasWidth)
		for x := 0; x < CanvasWidth; x++ {
			canvas[y][x] = Pixel{
				X:      x,
				Y:      y,
				Color:  "#ffffff",
				UserID: "",
			}
		}
	}
	return canvas
}

func saveCanvas(canvas [][]Pixel) error {
	db, err := getCanvasDB()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(canvas)
	return db.Put("data", data)
}

func loadCanvas() ([][]Pixel, error) {
	db, err := getCanvasDB()
	if err != nil {
		return createEmptyCanvas(), nil
	}
	data, err := db.Get("data")
	if err != nil {
		canvas := createEmptyCanvas()
		saveCanvas(canvas) // Ignore error
		return canvas, nil
	}
	if len(data) == 0 {
		canvas := createEmptyCanvas()
		saveCanvas(canvas) // Ignore error
		return canvas, nil
	}
	var canvas [][]Pixel
	err = json.Unmarshal(data, &canvas)
	if err != nil {
		canvas = createEmptyCanvas()
		saveCanvas(canvas) // Ignore error
		return canvas, nil
	}
	return canvas, nil
}

func saveUsers(users []User) error {
	db, err := getUsersDB()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(users)
	return db.Put("data", data)
}

func loadUsers() ([]User, error) {
	db, err := getUsersDB()
	if err != nil {
		return []User{}, nil
	}
	data, err := db.Get("data")
	if err != nil {
		return []User{}, nil
	}
	if len(data) == 0 {
		return []User{}, nil
	}
	var users []User
	err = json.Unmarshal(data, &users)
	if err != nil {
		return []User{}, nil
	}
	return users, nil
}

func saveMessages(messages []ChatMessage) error {
	db, err := getChatDB()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(messages)
	return db.Put("data", data)
}

func loadMessages() ([]ChatMessage, error) {
	db, err := getChatDB()
	if err != nil {
		return []ChatMessage{}, nil
	}
	data, err := db.Get("data")
	if err != nil {
		return []ChatMessage{}, nil
	}
	if len(data) == 0 {
		return []ChatMessage{}, nil
	}
	var messages []ChatMessage
	err = json.Unmarshal(data, &messages)
	if err != nil {
		return []ChatMessage{}, nil
	}
	return messages, nil
}

// ===== HTTP HANDLERS =====

//export getCanvas
func getCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	canvas, err := loadCanvas()
	if err != nil {
		return fail(h, err, 500)
	}

	data, err := json.Marshal(canvas)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Write(data)
	h.Return(200)
	return 0
}

//export getUsers
func getUsers(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	users, err := loadUsers()
	if err != nil {
		return fail(h, err, 500)
	}

	data, err := json.Marshal(users)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Write(data)
	h.Return(200)
	return 0
}

//export getMessages
func getMessages(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	messages, err := loadMessages()
	if err != nil {
		return fail(h, err, 500)
	}

	data, err := json.Marshal(messages)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Write(data)
	h.Return(200)
	return 0
}

//export getWebSocketURL
func getWebSocketURL(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	// get room from query
	room, err := h.Query().Get("room")
	if err != nil {
		room = "pixelupdates"
	}

	// hash the room to create a channel name
	hash := md5.New()
	hash.Write([]byte(room))
	roomHash := hex.EncodeToString(hash.Sum(nil))

	// create/open a channel with the hash
	channel, err := pubsub.Channel("pixel-" + roomHash)
	if err != nil {
		return fail(h, err, 500)
	}

	// get the websocket url
	url, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, err, 500)
	}

	// write the url to the response
	response := map[string]string{
		"websocket_url": url.Path,
	}

	data, err := json.Marshal(response)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Write(data)
	h.Return(200)
	return 0
}

//export resetCanvas
func resetCanvas(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	h.Headers().Set("Access-Control-Allow-Origin", "*")

	canvas := createEmptyCanvas()
	err = saveCanvas(canvas)
	if err != nil {
		return fail(h, err, 500)
	}

	h.Write([]byte("Canvas reset"))
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

	var pixel Pixel
	err = json.Unmarshal(data, &pixel)
	if err != nil {
		return 1
	}

	// Load current canvas
	canvas, err := loadCanvas()
	if err != nil {
		return 1
	}

	// Update canvas
	if pixel.X >= 0 && pixel.X < CanvasWidth && pixel.Y >= 0 && pixel.Y < CanvasHeight {
		canvas[pixel.Y][pixel.X] = pixel
		saveCanvas(canvas) // Ignore error
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

	// Load current users
	users, err := loadUsers()
	if err != nil {
		return 1
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

	saveUsers(users) // Ignore error
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

	// Load current messages
	messages, err := loadMessages()
	if err != nil {
		return 1
	}

	// Add message
	message.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	message.Timestamp = time.Now().Unix()
	messages = append(messages, message)

	// Keep only last 100 messages
	if len(messages) > 100 {
		messages = messages[len(messages)-100:]
	}

	saveMessages(messages) // Ignore error
	return 0
}

// ===== WEBSOCKET HANDLERS =====

//export onWebSocketMessage
func onWebSocketMessage(e event.Event) uint32 {
	channel, err := e.PubSub()
	if err != nil {
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		return 1
	}

	// Try to parse as different message types
	var pixel Pixel
	if err := json.Unmarshal(data, &pixel); err == nil && pixel.X >= 0 && pixel.Y >= 0 {
		// This is a pixel update
		// Publish to pixel channel
		pixelChannel, err := pubsub.Channel("pixelupdates")
		if err != nil {
			return 1
		}
		pixelData, _ := json.Marshal(pixel)
		pixelChannel.Publish(pixelData)
		return 0
	}

	var user User
	if err := json.Unmarshal(data, &user); err == nil && user.ID != "" {
		// This is a user update
		// Publish to user channel
		userChannel, err := pubsub.Channel("userupdates")
		if err != nil {
			return 1
		}
		userData, _ := json.Marshal(user)
		userChannel.Publish(userData)
		return 0
	}

	var message ChatMessage
	if err := json.Unmarshal(data, &message); err == nil && message.Message != "" {
		// This is a chat message
		// Publish to chat channel
		chatChannel, err := pubsub.Channel("chatmessages")
		if err != nil {
			return 1
		}
		messageData, _ := json.Marshal(message)
		chatChannel.Publish(messageData)
		return 0
	}

	return 1
}
