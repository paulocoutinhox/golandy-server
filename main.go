package main

import (
	"github.com/pborman/uuid"
	"log"
	"net/http"
	"sync"
	"fmt"
	"encoding/json"
	"time"
	"os"
	"io/ioutil"
	"golang.org/x/net/websocket"
)

var appVersion = "1.0.14"
var maps = make(map[string]*Map)

/*
var validateOrigin = false

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		if validateOrigin {
			if r.Header.Get("Origin") != "http://" + r.Host {
				return false
			}
		}

		return true
	},
}
*/

var Players = make([]*Player, 0)

type Map struct {
	Height       int `json:"height"`
	Layers       []struct {
		Data    []int  `json:"data"`
		Height  int    `json:"height"`
		Name    string `json:"name"`
		Opacity int    `json:"opacity"`
		Type    string `json:"type"`
		Visible bool   `json:"visible"`
		Width   int    `json:"width"`
		X       int    `json:"x"`
		Y       int    `json:"y"`
	} `json:"layers"`
	Nextobjectid int    `json:"nextobjectid"`
	Orientation  string `json:"orientation"`
	Renderorder  string `json:"renderorder"`
	Tileheight   int    `json:"tileheight"`
	Tilesets     []struct {
		Columns     int    `json:"columns"`
		Firstgid    int    `json:"firstgid"`
		Image       string `json:"image"`
		Imageheight int    `json:"imageheight"`
		Imagewidth  int    `json:"imagewidth"`
		Margin      int    `json:"margin"`
		Name        string `json:"name"`
		Spacing     int    `json:"spacing"`
		Tilecount   int    `json:"tilecount"`
		Tileheight  int    `json:"tileheight"`
		Tilewidth   int    `json:"tilewidth"`
	} `json:"tilesets"`
	Tilewidth    int `json:"tilewidth"`
	Version      int `json:"version"`
	Width        int `json:"width"`
}

type SimpleMessage struct {
	Type string `json:"type"`
}

type PongMessage struct {
	Type string `json:"type"`
	Time int64 `json:"time"`
}

type PlayerPositionMessage struct {
	Type      string `json:"type"`
	Id        string `json:"id"`
	X         int `json:"x"`
	Y         int `json:"y"`
	Direction int `json:"direction"`
}

type PlayerMoveOkMessage struct {
	Type      string `json:"type"`
	Id        string `json:"id"`
	X         int `json:"x"`
	Y         int `json:"y"`
	Direction int `json:"direction"`
}

type PlayerRemoveMessage struct {
	Type string `json:"type"`
	Id   string `json:"id"`
}

type PlayerInvalidPositionMessage struct {
	Type        string `json:"type"`
	Id          string `json:"id"`
	X           int `json:"x"`
	Y           int `json:"y"`
	Direction   int `json:"direction"`
	ToX         int `json:"toX"`
	ToY         int `json:"toY"`
	ToDirection int `json:"toDirection"`
}

type PlayerDataMessage struct {
	Type          string `json:"type"`
	Id            string `json:"id"`
	X             int `json:"x"`
	Y             int `json:"y"`
	CharType      string `json:"charType"`
	Direction     int `json:"direction"`
	MovementDelay int64 `json:"movementDelay"`
	Map           string `json:"map"`
}

type Player struct {
	Id               string
	X                int
	Y                int
	CharType         string
	Direction        int
	MovementDelay    int64
	LastMovementTime time.Time
	LastPingTime     time.Time
	Map              string

	Socket           *websocket.Conn
	mu               sync.Mutex
}

func debug(message string) {
	log.Printf("> %s\n", message)
}

/*
func randomInt(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max - min) + min
}
*/

func (p *Player) createSimpleMessage(messageType string) SimpleMessage {
	return SimpleMessage{Type: messageType}
}

func (p *Player) createPositionMessage(new bool) PlayerPositionMessage {
	return PlayerPositionMessage{Type: "move", X: p.X, Y: p.Y, Id: p.Id, Direction: p.Direction}
}

func (p *Player) createPlayerMoveOkMessage() PlayerMoveOkMessage {
	return PlayerMoveOkMessage{Type: "move-ok", X: p.X, Y: p.Y, Id: p.Id, Direction: p.Direction}
}

func (p *Player) createInvalidPositionMessage(toX, toY, toDirection int) PlayerInvalidPositionMessage {
	return PlayerInvalidPositionMessage{Type: "move-invalid", X: p.X, Y: p.Y, Id: p.Id, Direction: p.Direction, ToX: toX, ToY: toY, ToDirection: toDirection}
}

func (p *Player) createPlayerDataMessage() PlayerDataMessage {
	return PlayerDataMessage{Type: "player-data", X: p.X, Y: p.Y, Id: p.Id, CharType: p.CharType, Direction: p.Direction, MovementDelay: p.MovementDelay, Map: p.Map}
}

func (p *Player) createNewPlayerMessage() PlayerDataMessage {
	return PlayerDataMessage{Type: "player-new", X: p.X, Y: p.Y, Id: p.Id, CharType: p.CharType, Direction: p.Direction, MovementDelay: p.MovementDelay, Map: p.Map}
}

func (p *Player) createRemovePlayerMessage() PlayerRemoveMessage {
	return PlayerRemoveMessage{Type: "player-remove", Id: p.Id}
}

func (p *Player) createPongMessage() PongMessage {
	currentTime := time.Now().UTC()
	lastPingTime := p.LastPingTime
	diff := currentTime.Sub(lastPingTime).Nanoseconds() / int64(time.Millisecond)

	return PongMessage{Type: "pong", Time: diff}
}

func (p *Player) send(v interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	debug(fmt.Sprintf("Message sent: %v", v))
	return websocket.JSON.Send(p.Socket, v)
}

func (p *Player) sendToAll(v interface{}) {
	go func() {
		for _, player := range Players {
			if player.Id != p.Id {
				var err error

				if err = player.send(p.createPositionMessage(false)); err != nil {
					debug(fmt.Sprintf("Error on send command: %v", err))
				}
			}
		}
	}()
}

func (p *Player) updateLastMovementTime() {
	p.LastMovementTime = time.Now().UTC()
}

func (p *Player) updateLastPingTime() {
	p.LastPingTime = time.Now().UTC()
}

func (p *Player) canMoveTo(toX, toY, toDirection int) bool {
	// valida o tempo
	currentTime := time.Now().UTC()
	lastMovementTime := p.LastMovementTime
	diff := currentTime.Sub(lastMovementTime).Nanoseconds() / int64(time.Millisecond)

	if diff <= p.MovementDelay {
		debug(fmt.Sprintf("Player cannot move (movement delay) - %v, %v, %v", currentTime, lastMovementTime, diff))
		return false
	}

	// valida o tile
	var idx = toX + toY * maps[p.Map].Layers[0].Width
	var gid = maps[p.Map].Layers[0].Data[idx]

	if gid > 0 {
		debug("Player cannot move (map block)")
		return false
	}

	// valida a posição
	if (toX > (p.X + 1)) {
		debug("Player cannot move (invalid position - too far)")
		return false
	} else if (toX < (p.X - 1)) {
		debug("Player cannot move (invalid position - too far)")
		return false
	} else if (toY < (p.Y - 1)) {
		debug("Player cannot move (invalid position - too far)")
		return false
	} else if (toY > (p.Y + 1)) {
		debug("Player cannot move (invalid position - too far)")
		return false
	}

	return true
}

func wsHandler(ws *websocket.Conn) {
	// faz o upgrade da conexão pra websocket
	debug(fmt.Sprintf("New connection from: %+v", ws.RemoteAddr()))

	// cria o novo player
	player := new(Player)
	player.Id = uuid.New()
	player.Socket = ws

	player.CharType = "002"
	player.Direction = 3
	player.X = 3
	player.Y = 4
	player.MovementDelay = 200 //float64(randomInt(50, 200))
	player.LastMovementTime = time.Now().UTC()
	player.LastPingTime = time.Now().UTC()
	player.Map = "001"

	// listen para comandos ou erros
	for {
		messageRaw := make([]byte, 512)
		messageLength, err := ws.Read(messageRaw)
		message := messageRaw[:messageLength]

		if err != nil {
			debug(fmt.Sprintf("Error on player: %v", err))

			// +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
			// erro no socket e foi desconectado - envia essa informação para todos
			// +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

			for i, p := range Players {
				if p.Id == player.Id {
					Players = append(Players[:i], Players[i + 1:]...)
				} else {
					debug(fmt.Sprintf("Destroy player: %v", player))

					if err = p.send(player.createRemovePlayerMessage()); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}
				}
			}

			debug(fmt.Sprintf("Players connected: %v", len(Players)))

			break
		}

		debug(fmt.Sprintf("Message received: %v - %v", string(message), messageLength))

		var messageData map[string]interface{}

		if err := json.Unmarshal(message, &messageData); err != nil {
			debug(fmt.Sprintf("Erro while decode message: %v", err))
		} else {
			messageDataType := messageData["type"]

			if messageDataType == "ping" {
				// ++++++++++++++++++++++++++++++++++++++++++
				// ping - comando para validar o delay no cliente
				// ++++++++++++++++++++++++++++++++++++++++++
				if err = player.send(player.createPongMessage()); err != nil {
					debug(fmt.Sprintf("Error on send command: %v", err))
				}

				player.updateLastPingTime()
			} else if messageDataType == "move" {
				// ++++++++++++++++++++++++++++++++++++++++++
				// move = posição do personagem
				// ++++++++++++++++++++++++++++++++++++++++++

				var toX, toY, toDirection int

				if value, ok := messageData["x"]; ok {
					toX = int(value.(float64))
				}

				if value, ok := messageData["y"]; ok {
					toY = int(value.(float64))
				}

				if value, ok := messageData["direction"]; ok {
					toDirection = int(value.(float64))
				}

				if player.canMoveTo(toX, toY, toDirection) {
					debug(player.LastMovementTime.String())
					debug(time.Now().UTC().String())
					player.updateLastMovementTime()

					player.X = toX
					player.Y = toY
					player.Direction = toDirection

					if err = player.send(player.createPlayerMoveOkMessage()); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}

					player.sendToAll(player.createPositionMessage(false))
				} else {
					if err = player.send(player.createInvalidPositionMessage(toX, toY, toDirection)); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}
				}
			} else if messageDataType == "login" {
				// ++++++++++++++++++++++++++++++++++++++++++
				// login = pedido de login
				// ++++++++++++++++++++++++++++++++++++++++++

				var username = ""
				var password = ""
				var version = ""

				if value, ok := messageData["username"]; ok {
					username = value.(string)
				}

				if value, ok := messageData["password"]; ok {
					password = value.(string)
				}

				if value, ok := messageData["version"]; ok {
					version = value.(string)
				}

				if version != appVersion {
					debug(fmt.Sprintf("Player is trying use a different version: %v", version))

					if err = player.send(player.createSimpleMessage("version-invalid")); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}

					player.Socket.Close()
				}

				if username == "demo" && password == "demo" {
					// cria o novo player
					debug(fmt.Sprintf("New player logged: %v - %v", username, password))

					Players = append(Players, player)
					debug(fmt.Sprintf("New player: %v", player))

					if err = player.send(player.createSimpleMessage("login-ok")); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}
				} else {
					debug(fmt.Sprintf("Player is trying do login with a invalid username and password: %v - %v", username, password))

					if err = player.send(player.createSimpleMessage("login-invalid")); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}

					player.Socket.Close()
				}
			} else if messageDataType == "game-data" {
				// ++++++++++++++++++++++++++++++++++++++++++
				// game-data = dados do jogo
				// ++++++++++++++++++++++++++++++++++++++++++
				debug("Sending player data...")

				if err = player.send(player.createPlayerDataMessage()); err != nil {
					debug(fmt.Sprintf("Error on send command: %v", err))
				}

				debug("Sent")

				// envia a posição do novo player para todos
				debug("Publishing positions...")

				go func() {
					for _, p := range Players {
						if p.Id != player.Id {
							if err = player.send(p.createNewPlayerMessage()); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}

							if err = p.send(player.createNewPlayerMessage()); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}
						}
					}
				}()

				debug("Published")
			}
		}

		/*
		go func() {
			for _, p := range Players {
				if p.Id != player.Id {
					if err = p.send(player.createPositionMessage(false)); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}
				}

			}
		}()
		*/
	}
}

func loadMaps() {
	file, err := ioutil.ReadFile("maps/map1.json")

	if err != nil {
		debug(fmt.Sprintf("Failed to load map: %v", err))
		os.Exit(1)
	}

	var m Map
	json.Unmarshal(file, &m)

	maps["001"] = &m

	for _, currentMap := range maps {
		for currentLayerKey, currentLayer := range currentMap.Layers {
			if currentLayer.Name != "Meta" {
				currentMap.Layers = append(currentMap.Layers[:currentLayerKey], currentMap.Layers[currentLayerKey + 1:]...)
			}
		}
	}
}

func main() {
	loadMaps()

	/*
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/ws", func(c *gin.Context) {
		handler := websocket.Handler(wsHandler)
		handler.ServeHTTP(c.Writer, c.Request)

		wsHandler(c.Writer, c.Request)
	})

	r.Static("/static", "public")

	r.Run(":3030")
	*/

	http.Handle("/ws", websocket.Handler(wsHandler))
	http.Handle("/public", http.FileServer(http.Dir("public")))

	err := http.ListenAndServe(":3030", nil)

	if err != nil {
		debug("Fatal Error: " + err.Error())
		os.Exit(1)
	}
}
