//go:build !wasm && !wasi

package lib

func init() {
	placePixel(0)
	joinGame(0)
	leaveGame(0)
	getCanvas(0)
	getUsers(0)
	sendMessage(0)
	getMessages(0)
	onPixelUpdate(0)
	onUserUpdate(0)
	onChatMessage(0)
	getWebSocketURL(0)
	initCanvas(0)
}
