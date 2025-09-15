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

// ===== UTILITY FUNCTIONS =====
func fail(h http.Event, err error, code int) uint32 {
	h.Write([]byte(err.Error()))
	h.Return(code)
	return 1
}

func setCORSHeaders(h http.Event) {
	h.Headers().Set("Access-Control-Allow-Origin", "*")
	h.Headers().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	h.Headers().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// ===== HTTP HANDLERS =====

//export ws
func ws(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// get room from query
	room, err := h.Query().Get("room")
	if err != nil {
		room = "default"
	}

	// create/open channel with room-specific name
	channel, err := pubsub.Channel("game-" + room)
	if err != nil {
		return fail(h, err, 500)
	}

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

//export onGameMessage
func onGameMessage(e event.Event) uint32 {
	channel, err := e.PubSub()
	if err != nil {
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		return 1
	}

	// Try to parse as pixel update first
	var pixelUpdate struct {
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Color    string `json:"color"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Room     string `json:"room"`
	}

	err = json.Unmarshal(data, &pixelUpdate)
	if err == nil && pixelUpdate.X >= 0 && pixelUpdate.Y >= 0 {
		// This is a pixel update
		room := pixelUpdate.Room
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
			err = db.Put(canvasKey, updatedData)
			if err != nil {
				return 1
			}

			// Broadcast pixel update to all connected clients via WebSocket
			gameChannel, err := pubsub.Channel("game-" + room)
			if err == nil {
				pixelData, _ := json.Marshal(pixelUpdate)
				gameChannel.Publish(pixelData)
			}
		}
		return 0
	}

	// Try to parse as chat message
	var message struct {
		Message  string `json:"message"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Room     string `json:"room"`
	}
	err = json.Unmarshal(data, &message)
	if err == nil && message.Message != "" {
		// This is a chat message
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
		}

		var messages []ChatMessage
		err = json.Unmarshal(messagesData, &messages)
		if err != nil {
			messages = []ChatMessage{}
		}

		// Add message with timestamp
		chatMessage := ChatMessage{
			ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
			UserID:    message.UserID,
			Username:  message.Username,
			Message:   message.Message,
			Timestamp: time.Now().Unix(),
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

		// Broadcast chat message to all connected clients via WebSocket
		gameChannel, err := pubsub.Channel("game-" + room)
		if err == nil {
			messageData, _ := json.Marshal(chatMessage)
			gameChannel.Publish(messageData)
		}
		return 0
	}

	return 1
}

//export sendPixel
func sendPixel(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// Read the request body
	body := h.Body()
	bodyData := make([]byte, 1024)
	n, err := body.Read(bodyData)
	if err != nil {
		return fail(h, err, 400)
	}
	bodyData = bodyData[:n]

	var pixelUpdate struct {
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Color    string `json:"color"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Room     string `json:"room"`
	}

	err = json.Unmarshal(bodyData, &pixelUpdate)
	if err != nil {
		return fail(h, err, 400)
	}

	// Use room from message
	room := pixelUpdate.Room
	if room == "" {
		room = "default"
	}

	// Update canvas in database
	db, err := database.New("/canvas")
	if err != nil {
		return fail(h, err, 500)
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
	}

	var canvas [][]string
	err = json.Unmarshal(canvasData, &canvas)
	if err != nil {
		return fail(h, err, 500)
	}

	// Update pixel
	if pixelUpdate.X >= 0 && pixelUpdate.X < CanvasWidth &&
		pixelUpdate.Y >= 0 && pixelUpdate.Y < CanvasHeight {
		canvas[pixelUpdate.Y][pixelUpdate.X] = pixelUpdate.Color

		// Save updated canvas
		updatedData, err := json.Marshal(canvas)
		if err != nil {
			return fail(h, err, 500)
		}
		err = db.Put(canvasKey, updatedData)
		if err != nil {
			return fail(h, err, 500)
		}

		// Publish to channel to trigger onPixelUpdate function
		pixelChannel, err := pubsub.Channel("pixelupdates")
		if err == nil {
			pixelData, _ := json.Marshal(pixelUpdate)
			pixelChannel.Publish(pixelData)
		}
	}

	h.Return(200)
	return 0
}

//export sendMessage
func sendMessage(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// Read the request body
	body := h.Body()
	bodyData := make([]byte, 1024)
	n, err := body.Read(bodyData)
	if err != nil {
		return fail(h, err, 400)
	}
	bodyData = bodyData[:n]

	var message struct {
		Message  string `json:"message"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Room     string `json:"room"`
	}
	err = json.Unmarshal(bodyData, &message)
	if err != nil {
		return fail(h, err, 400)
	}

	// Use room from message
	room := message.Room
	if room == "" {
		room = "default"
	}

	// Update messages in database
	db, err := database.New("/chat")
	if err != nil {
		return fail(h, err, 500)
	}

	chatKey := "room:" + room
	messagesData, err := db.Get(chatKey)
	if err != nil {
		messagesData = []byte("[]")
	}

	var messages []ChatMessage
	err = json.Unmarshal(messagesData, &messages)
	if err != nil {
		messages = []ChatMessage{}
	}

	// Add message with timestamp
	chatMessage := ChatMessage{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		UserID:    message.UserID,
		Username:  message.Username,
		Message:   message.Message,
		Timestamp: time.Now().Unix(),
	}
	messages = append(messages, chatMessage)

	// Keep only last 100 messages
	if len(messages) > 100 {
		messages = messages[len(messages)-100:]
	}

	// Save updated messages
	updatedData, err := json.Marshal(messages)
	if err != nil {
		return fail(h, err, 500)
	}
	err = db.Put(chatKey, updatedData)
	if err != nil {
		return fail(h, err, 500)
	}

	// Publish to channel to trigger onChatMessage function
	chatChannel, err := pubsub.Channel("chatmessages")
	if err == nil {
		messageData, _ := json.Marshal(chatMessage)
		chatChannel.Publish(messageData)
	}

	h.Return(200)
	return 0
}
