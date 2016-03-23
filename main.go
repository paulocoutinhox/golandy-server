package main

import (
	"github.com/pborman/uuid"
	"github.com/gorilla/websocket"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"sync"
	"fmt"
	"encoding/json"
	"time"
)

var appVersion = "1.0"
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

var Players = make([]*Player, 0)

type SimpleMessage struct {
	Type string `json:"type"`
}

type PlayerPositionMessage struct {
	Type   string `json:"type"`
	Id     string `json:"id"`
	X      int `json:"x"`
	Y      int `json:"y"`
	New    bool `json:"new"`
	Online bool `json:"online"`
}

type PlayerRemoveMessage struct {
	Type string `json:"type"`
	Id   string `json:"id"`
}

type PlayerInvalidPositionMessage struct {
	Type string `json:"type"`
	Id   string `json:"id"`
	X    int `json:"x"`
	Y    int `json:"y"`
}

type PlayerDataMessage struct {
	Type          string `json:"type"`
	Id            string `json:"id"`
	X             int `json:"x"`
	Y             int `json:"y"`
	CharType      string `json:"charType"`
	Direction     int `json:"direction"`
	MovementDelay float64 `json:"movementDelay"`
}

type Player struct {
	Id               string
	X                int
	Y                int
	CharType         string
	Direction        int
	MovementDelay    float64
	LastMovementTime time.Time

	Socket           *websocket.Conn
	mu               sync.Mutex
}

func debug(message string) {
	log.Printf("> %s\n", message);
}

func (p *Player) createSimpleMessage(messageType string) SimpleMessage {
	return SimpleMessage{Type: messageType}
}

func (p *Player) createPositionMessage(new bool) PlayerPositionMessage {
	return PlayerPositionMessage{Type: "pos", X: p.X, Y: p.Y, Id: p.Id, New: new, Online: true}
}

func (p *Player) createInvalidPositionMessage() PlayerInvalidPositionMessage {
	return PlayerInvalidPositionMessage{Type: "pos-invalid", X: p.X, Y: p.Y, Id: p.Id}
}

func (p *Player) createPlayerDataMessage() PlayerDataMessage {
	return PlayerDataMessage{Type: "player-data", X: p.X, Y: p.Y, Id: p.Id, CharType: p.CharType, Direction: p.Direction, MovementDelay: p.MovementDelay}
}

func (p *Player) createNewPlayerMessage() PlayerDataMessage {
	return PlayerDataMessage{Type: "player-new", X: p.X, Y: p.Y, Id: p.Id, CharType: p.CharType, Direction: p.Direction, MovementDelay: p.MovementDelay}
}

func (p *Player) createRemovePlayerMessage() PlayerRemoveMessage {
	return PlayerRemoveMessage{Type: "player-remove", Id: p.Id}
}

func (p *Player) send(v interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Socket.WriteJSON(v)
}

func (p *Player) updateLastMovementTime() {
	p.LastMovementTime = time.Now()
}

func (p *Player) canMove() bool {
	seconds := time.Now().Sub(p.LastMovementTime).Seconds()
	ms := seconds * 1000
	return (ms > p.MovementDelay)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	// faz o upgrade da conexão pra websocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)

	if err != nil {
		debug(fmt.Sprintf("Failed to set websocket upgrade: %v", err))
		return
	}

	debug(fmt.Sprintf("New connection from: %+v", conn.RemoteAddr()))

	// cria o novo player
	player := new(Player)
	player.Id = uuid.New()
	player.Socket = conn

	player.CharType = "002"
	player.Direction = 3
	player.X = 3;
	player.Y = 4;
	player.MovementDelay = 200;
	player.LastMovementTime = time.Now();

	// listen para comandos ou erros
	for {
		messageType, message, err := conn.ReadMessage()

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

		debug(fmt.Sprintf("Message received: %v - %v", messageType, string(message)))

		var messageData map[string]interface{}

		if err := json.Unmarshal(message, &messageData); err != nil {
			debug(fmt.Sprintf("Erro while decode message: %v", err))
		} else {
			messageDataType := messageData["type"]

			if messageDataType == "pos" {
				// ++++++++++++++++++++++++++++++++++++++++++
				// pos = posição do personagem
				// ++++++++++++++++++++++++++++++++++++++++++

				if player.canMove() {
					player.updateLastMovementTime()

					if value, ok := messageData["x"]; ok {
						player.X = int(value.(float64))
					}

					if value, ok := messageData["y"]; ok {
						player.Y = int(value.(float64))
					}

					if err = player.send(player.createSimpleMessage("pos-ok")); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}
				} else {
					if err = player.send(player.createInvalidPositionMessage()); err != nil {
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

					if err = player.send(player.createPlayerDataMessage()); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}

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
				} else {
					debug(fmt.Sprintf("Player is trying do login with a invalid username and password: %v - %v", username, password))

					if err = player.send(player.createSimpleMessage("login-invalid")); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}

					player.Socket.Close()
				}
			}
		}

		go func() {
			for _, p := range Players {
				if p.Id != player.Id {
					if err = p.send(player.createPositionMessage(false)); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}
				}

			}
		}()
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/ws", func(c *gin.Context) {
		wsHandler(c.Writer, c.Request)
	})

	r.Static("/static", "public")

	r.Run(":3030")
}
