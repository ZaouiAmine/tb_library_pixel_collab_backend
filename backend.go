package lib

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"

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

	room, err := h.Query().Get("room")
	if err != nil {
		room = "pixelupdates"
	}

	hash := md5.New()
	hash.Write([]byte(room))
	roomHash := hex.EncodeToString(hash.Sum(nil))

	channel, err := pubsub.Channel("pixel" + roomHash)
	if err != nil {
		return fail(h, err, 500)
	}

	url, err := channel.WebSocket().Url()
	if err != nil {
		return fail(h, err, 500)
	}

	response := map[string]string{
		"websocket_url": url.Path,
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

//export resetCanvas
func resetCanvas(e event.Event) uint32 {
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
