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
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// valida a origem - desabilitado por enquanto
		/*
		if r.Header.Get("Origin") != "http://" + r.Host {
			return false
		}
		*/
		return true
	},
}

var Players = make([]*Player, 0)

type Message struct {
	Y      int    // current Y position
	X      int    // as above
	Id     string // the id of the player that sent the message
	New    bool   // true if this player just connected so we know when to
                  // spawn a new sprite on the screens of the other players. for all subsequent
                  // messages it's false
	Online bool   // true if the player is no longer connected so the frontend
                  // will remove it's sprite
}

type Player struct {
	Y      int             // Y position of the player
	X      int             // X position
	Id     string          // a unique id to identify the player by the frontend
	Socket *websocket.Conn // websocket connection of the player
	mu     sync.Mutex
}

func debug(message string) {
	log.Printf("> %s\n", message);
}

func (p *Player) position(new bool) Message {
	return Message{X: p.X, Y: p.Y, Id: p.Id, New: new, Online: true}
}

func (p *Player) send(v interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Socket.WriteJSON(v)
}

// a slice of *Players which will store the list of connected players
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

	Players = append(Players, player)

	debug(fmt.Sprintf("New player: %v", player))

	// envia a posição do novo player para todos
	debug("Publishing positions...")

	go func() {
		for _, p := range Players {
			if p.Socket.RemoteAddr() != player.Socket.RemoteAddr() {
				if err = player.send(p.position(true)); err != nil {
					debug(fmt.Sprintf("Error on send command: %v", err))
				}

				if err = p.send(player.position(true)); err != nil {
					debug(fmt.Sprintf("Error on send command: %v", err))
				}
			}
		}
	}()

	debug("Published")

	for {
		messageType, message, err := conn.ReadMessage()

		if err != nil {
			debug(fmt.Sprintf("Error on player: %v", err))

			// erro no socket e foi desconectado - envia essa informação para todos
			for i, p := range Players {
				if p.Id == player.Id {
					Players = append(Players[:i], Players[i + 1:]...)
				} else {
					debug(fmt.Sprintf("Destroy player: %v", player))

					if err = p.send(Message{Online: false, Id: player.Id}); err != nil {
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
				if value, ok := messageData["x"]; ok {
					player.X = int(value.(float64))
				}

				if value, ok := messageData["y"]; ok {
					player.Y = int(value.(float64))
				}
			}
		}

		go func() {
			for _, p := range Players {
				if p.Id != player.Id {
					if err = p.send(player.position(false)); err != nil {
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
