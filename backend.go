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
	CanvasWidth  = 100
	CanvasHeight = 100
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
		emptyCanvas := make([][]Pixel, CanvasHeight)
		for y := 0; y < CanvasHeight; y++ {
			emptyCanvas[y] = make([]Pixel, CanvasWidth)
			for x := 0; x < CanvasWidth; x++ {
				emptyCanvas[y][x] = Pixel{
					X:      x,
					Y:      y,
					Color:  "#ffffff",
					UserID: "",
				}
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

//export getWebSocketURL
func getWebSocketURL(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}

	// create/open a channel with hardcoded name
	channel, err := pubsub.Channel("pixelupdates")
	if err != nil {
		return fail(h, err, 500)
	}

	// get the websocket url
	url, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, err, 500)
	}

	// write the url to the response
	h.Write([]byte(url.Path))

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

	emptyCanvas := make([][]Pixel, CanvasHeight)
	for y := 0; y < CanvasHeight; y++ {
		emptyCanvas[y] = make([]Pixel, CanvasWidth)
		for x := 0; x < CanvasWidth; x++ {
			emptyCanvas[y][x] = Pixel{
				X:      x,
				Y:      y,
				Color:  "#ffffff",
				UserID: "",
			}
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

	h.Write([]byte("Canvas reset"))
	h.Return(200)
	return 0
}

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
	db, err := database.New("/canvas")
	if err != nil {
		return 1
	}

	canvasData, err := db.Get("data")
	if err != nil {
		return 1
	}

	var canvas [][]Pixel
	err = json.Unmarshal(canvasData, &canvas)
	if err != nil {
		return 1
	}

	// Update canvas
	if pixel.X >= 0 && pixel.X < CanvasWidth && pixel.Y >= 0 && pixel.Y < CanvasHeight {
		canvas[pixel.Y][pixel.X] = pixel

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

	// Load current users
	db, err := database.New("/users")
	if err != nil {
		return 1
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

	// Load current messages
	db, err := database.New("/chat")
	if err != nil {
		return 1
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
		return 1
	}
	err = db.Put("data", updatedData)
	if err != nil {
		return 1
	}

	return 0
}
