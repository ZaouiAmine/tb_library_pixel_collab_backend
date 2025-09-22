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

//export getPixelChannelURL
func getPixelChannelURL(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// create/open pixel channel with fixed name
	channel, err := pubsub.Channel("pixelupdates")
	if err != nil {
		return fail(h, err, 500)
	}
	channel.Subscribe()
	// get the websocket url
	url, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, err, 500)
	}
	fmt.Println("pixelchannel..........................")
	// Return the WebSocket path directly as a string
	h.Headers().Set("Content-Type", "text/plain")
	h.Write([]byte(url.Path))
	h.Return(200)
	return 0
}

//export getChatChannelURL
func getChatChannelURL(e event.Event) uint32 {
	h, err := e.HTTP()
	if err != nil {
		return 1
	}
	setCORSHeaders(h)

	// create/open chat channel with fixed name
	channel, err := pubsub.Channel("chatmessages")
	if err != nil {
		return fail(h, err, 500)
	}
	channel.Subscribe()
	// get the websocket url
	url, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, err, 500)
	}
	fmt.Println("here................................")
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

//export onPixelUpdate
func onPixelUpdate(e event.Event) uint32 {
	fmt.Println("=== onPixelUpdate triggered ===")
	fmt.Println("Event type:", fmt.Sprintf("%T", e))

	channel, err := e.PubSub()
	if err != nil {
		fmt.Println("Error getting PubSub channel:", err)
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		fmt.Println("Error getting channel data:", err)
		return 1
	}

	fmt.Println("Raw channel data type:", fmt.Sprintf("%T", data))
	fmt.Println("Raw channel data length:", len(data))
	fmt.Println("Raw channel data bytes:", data)
	fmt.Println("Received pixel data as string:", string(data))

	var pixelUpdate struct {
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Color    string `json:"color"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Room     string `json:"room"`
	}

	err = json.Unmarshal(data, &pixelUpdate)
	if err != nil {
		fmt.Println("Error parsing pixel data:", err)
		return 1
	}

	fmt.Printf("Parsed pixel: x=%d, y=%d, color=%s, room=%s\n",
		pixelUpdate.X, pixelUpdate.Y, pixelUpdate.Color, pixelUpdate.Room)

	// Use room from message
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
		//pixelChannel, err := pubsub.Channel("pixelupdates")
		//if err == nil {
		//	pixelData, _ := json.Marshal(pixelUpdate)
		//	pixelChannel.Publish(pixelData)
		//	fmt.Println("Broadcasted pixel update to channel")
		//} else {
		//	fmt.Println("Error getting pixel channel for broadcast:", err)
		//}
	} else {
		fmt.Println("Pixel coordinates out of bounds")
	}

	fmt.Println("=== onPixelUpdate completed ===")
	return 0
}

//export onChatMessage
func onChatMessages(e event.Event) uint32 {
	fmt.Println("=== onChatMessage triggered ===")
	fmt.Println("Event type:", fmt.Sprintf("%T", e))

	channel, err := e.PubSub()
	if err != nil {
		fmt.Println("Error getting PubSub channel:", err)
		return 1
	}

	data, err := channel.Data()
	if err != nil {
		fmt.Println("Error getting channel data:", err)
		return 1
	}

	fmt.Println("Raw chat channel data type:", fmt.Sprintf("%T", data))
	fmt.Println("Raw chat channel data length:", len(data))
	fmt.Println("Raw chat channel data bytes:", data)
	fmt.Println("Received chat data as string:", string(data))

	var message struct {
		Message  string `json:"message"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Room     string `json:"room"`
	}
	err = json.Unmarshal(data, &message)
	if err != nil {
		return 1
	}

	// Use room from message
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
	chatChannel, err := pubsub.Channel("chatmessages")
	if err == nil {
		messageData, _ := json.Marshal(chatMessage)
		chatChannel.Publish(messageData)
	}

	return 0
}
