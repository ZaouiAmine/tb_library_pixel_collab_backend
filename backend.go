package lib

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/taubyte/go-sdk/database"
	"github.com/taubyte/go-sdk/event"
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

type Canvas struct {
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Pixels [][]Pixel `json:"pixels"`
}

// ===== CONSTANTS =====
const (
	CanvasWidth  = 100
	CanvasHeight = 100
)

// ===== GLOBAL STATE =====
var (
	canvas   [][]Pixel
	users    []User
	messages []ChatMessage
)

// ===== UTILITY FUNCTIONS =====
func getDB() database.Database {
	db, _ := database.New("/pixel_collab")
	return db
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

func saveCanvas() {
	db := getDB()
	data, _ := json.Marshal(canvas)
	db.Put("canvas", data)
}

func loadCanvas() {
	db := getDB()
	data, err := db.Get("canvas")
	if err != nil {
		canvas = createEmptyCanvas()
		saveCanvas()
		return
	}
	if len(data) == 0 {
		canvas = createEmptyCanvas()
		saveCanvas()
		return
	}
	json.Unmarshal(data, &canvas)
}

func saveUsers() {
	db := getDB()
	data, _ := json.Marshal(users)
	db.Put("users", data)
}

func loadUsers() {
	db := getDB()
	data, err := db.Get("users")
	if err != nil {
		users = []User{}
		return
	}
	if len(data) == 0 {
		users = []User{}
		return
	}
	json.Unmarshal(data, &users)
}

func saveMessages() {
	db := getDB()
	data, _ := json.Marshal(messages)
	db.Put("messages", data)
}

func loadMessages() {
	db := getDB()
	data, err := db.Get("messages")
	if err != nil {
		messages = []ChatMessage{}
		return
	}
	if len(data) == 0 {
		messages = []ChatMessage{}
		return
	}
	json.Unmarshal(data, &messages)
}

func init() {
	loadCanvas()
	loadUsers()
	loadMessages()
}

// ===== HTTP HANDLERS =====

//export getCanvas
func getCanvas(e event.Event) uint32 {
	h, _ := e.HTTP()
	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	data, _ := json.Marshal(canvas)
	h.Write(data)
	h.Return(200)
	return 0
}

//export getUsers
func getUsers(e event.Event) uint32 {
	h, _ := e.HTTP()
	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	data, _ := json.Marshal(users)
	h.Write(data)
	h.Return(200)
	return 0
}

//export getMessages
func getMessages(e event.Event) uint32 {
	h, _ := e.HTTP()
	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	data, _ := json.Marshal(messages)
	h.Write(data)
	h.Return(200)
	return 0
}

//export getWebSocketURL
func getWebSocketURL(e event.Event) uint32 {
	h, _ := e.HTTP()
	h.Headers().Set("Content-Type", "application/json")
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	room, _ := h.Query().Get("room")
	if room == "" {
		room = "pixelupdates"
	}

	channel, _ := pubsub.Channel(room)
	wsURL, _ := channel.WebSocket().Url()

	response := map[string]string{
		"websocket_url": wsURL.Path,
	}

	data, _ := json.Marshal(response)
	h.Write(data)
	h.Return(200)
	return 0
}

//export initCanvas
func initCanvas(e event.Event) uint32 {
	h, _ := e.HTTP()
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	canvas = createEmptyCanvas()
	saveCanvas()

	h.Write([]byte("Canvas initialized"))
	h.Return(200)
	return 0
}

//export resetCanvas
func resetCanvas(e event.Event) uint32 {
	h, _ := e.HTTP()
	h.Headers().Set("Access-Control-Allow-Origin", "*")

	canvas = createEmptyCanvas()
	saveCanvas()

	h.Write([]byte("Canvas reset"))
	h.Return(200)
	return 0
}

// ===== PUB/SUB HANDLERS =====

//export onPixelUpdate
func onPixelUpdate(e event.Event) uint32 {
	channel, _ := e.PubSub()
	data, _ := channel.Data()

	var pixel Pixel
	json.Unmarshal(data, &pixel)

	// Update canvas
	if pixel.X >= 0 && pixel.X < CanvasWidth && pixel.Y >= 0 && pixel.Y < CanvasHeight {
		canvas[pixel.Y][pixel.X] = pixel
		saveCanvas()
	}

	return 0
}

//export onUserUpdate
func onUserUpdate(e event.Event) uint32 {
	channel, _ := e.PubSub()
	data, _ := channel.Data()

	var user User
	json.Unmarshal(data, &user)

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

	saveUsers()
	return 0
}

//export onChatMessage
func onChatMessage(e event.Event) uint32 {
	channel, _ := e.PubSub()
	data, _ := channel.Data()

	var message ChatMessage
	json.Unmarshal(data, &message)

	// Add message
	message.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	message.Timestamp = time.Now().Unix()
	messages = append(messages, message)

	// Keep only last 100 messages
	if len(messages) > 100 {
		messages = messages[len(messages)-100:]
	}

	saveMessages()
	return 0
}
