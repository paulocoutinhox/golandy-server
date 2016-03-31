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
	"math/rand"
)

var appVersion = "1.0.17"
var maps = make(map[string]*Map)
var tickerBombs = time.NewTicker(time.Millisecond * 500)
var playersMU sync.Mutex
var bombsMU sync.Mutex

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
var Bombs = make([]*Bomb, 0)

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

type PlayerRemovedMessage struct {
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
	AddBombDelay  int64 `json:"addBombDelay"`
}

type BombAddedMessage struct {
	Type          string `json:"type"`
	Id            string `json:"id"`
	X             int `json:"x"`
	Y             int `json:"y"`
	BombType      string `json:"bombType"`
	Direction     int `json:"direction"`
	MovementDelay int64 `json:"movementDelay"`
	CreatedAt     int64 `json:"createdAt"`
	FireDelay     int64 `json:"fireDelay"`
	FireLength    int `json:"fireLength"`
	Player        string `json:"player"`
}

type BombAddInvalidMessage struct {
	Type string `json:"type"`
	X    int `json:"x"`
	Y    int `json:"y"`
	ToX  int `json:"toX"`
	ToY  int `json:"toY"`
}

type BombFiredMessage struct {
	Type       string `json:"type"`
	Id         string `json:"id"`
	X          int `json:"x"`
	Y          int `json:"y"`
	BombType   string `json:"bombType"`
	Direction  int `json:"direction"`
	FireLength int `json:"fireLength"`
	Player     string `json:"player"`
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
	LastAddBombTime  time.Time
	AddBombDelay     int64
	Online           bool

	Socket           *websocket.Conn
	mu               sync.Mutex
}

type Bomb struct {
	Id               string
	X                int
	Y                int
	BombType         string
	Direction        int
	MovementDelay    int64
	LastMovementTime time.Time
	CreatedAt        time.Time
	FireLength       int
	FireDelay        int64
	Player           *Player
}

type Point struct {
	X int
	Y int
}

func debug(message string) {
	log.Printf("> %s\n", message)
}

func getMillisecondsFromTime(fullTime time.Time) int64 {
	return int64(fullTime.Nanosecond() / int(time.Millisecond))
}

func removePlayer(player *Player) {
	playersMU.Lock()
	defer playersMU.Unlock()

	for i, p := range Players {
		if p.Id == player.Id {
			Players = append(Players[:i], Players[i + 1:]...)
		}
	}
}

func addPlayer(player *Player) {
	playersMU.Lock()
	defer playersMU.Unlock()

	Players = append(Players, player)
}

func removeBomb(bomb *Bomb) {
	bombsMU.Lock()
	defer bombsMU.Unlock()

	for i, b := range Bombs {
		if b.Id == bomb.Id {
			Bombs = append(Bombs[:i], Bombs[i + 1:]...)
		}
	}
}

func addBomb(bomb *Bomb) {
	bombsMU.Lock()
	defer bombsMU.Unlock()

	Bombs = append(Bombs, bomb)
}

func isTileBlocking(mapType string, x, y int) bool {
	bombsMU.Lock()
	defer bombsMU.Unlock()

	var idx = x + y * maps[mapType].Layers[0].Width
	var gid = maps[mapType].Layers[0].Data[idx]

	if gid > 0 {
		return true
	}

	return false
}

func inPointList(desiredX, desiredY int, list []*Point) bool {
	for _, point := range list {
		if point.X == desiredX && point.Y == desiredY {
			return true
		}
	}

	return false
}

func randomInt(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max - min) + min
}

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

func (p *Player) createPlayerAddedMessage() PlayerDataMessage {
	return PlayerDataMessage{Type: "player-added", X: p.X, Y: p.Y, Id: p.Id, CharType: p.CharType, Direction: p.Direction, MovementDelay: p.MovementDelay, Map: p.Map}
}

func (p *Player) createPlayerDeadMessage() PlayerDataMessage {
	return PlayerDataMessage{Type: "player-dead", X: p.X, Y: p.Y, Id: p.Id, CharType: p.CharType, Direction: p.Direction, MovementDelay: p.MovementDelay, Map: p.Map}
}

func (p *Player) createPlayerRemovedMessage() PlayerRemovedMessage {
	return PlayerRemovedMessage{Type: "player-removed", Id: p.Id}
}

func (p *Player) createPongMessage() PongMessage {
	currentTime := time.Now().UTC()
	lastPingTime := p.LastPingTime
	diff := currentTime.Sub(lastPingTime).Nanoseconds() / int64(time.Millisecond)

	return PongMessage{Type: "pong", Time: diff}
}

func (p *Player) createBombAddedMessage(bomb *Bomb) BombAddedMessage {
	playerID := ""

	if bomb.Player != nil {
		playerID = bomb.Player.Id
	}

	return BombAddedMessage{Type: "bomb-added", Id: bomb.Id, X: bomb.X, Y: bomb.Y, BombType: bomb.BombType, Direction: bomb.Direction, MovementDelay: bomb.MovementDelay, CreatedAt: getMillisecondsFromTime(bomb.CreatedAt), FireDelay: bomb.FireDelay, FireLength: bomb.FireLength, Player: playerID}
}

func (p *Player) createBombFiredMessage(bomb *Bomb) BombFiredMessage {
	playerID := ""

	if bomb.Player != nil {
		playerID = bomb.Player.Id
	}

	return BombFiredMessage{Type: "bomb-fired", Id: bomb.Id, X: bomb.X, Y: bomb.Y, BombType: bomb.BombType, Direction: bomb.Direction, FireLength: bomb.FireLength, Player: playerID}
}

func (p *Player) createBombAddInvalidMessage(bombX, bombY int) BombAddInvalidMessage {
	return BombAddInvalidMessage{Type: "bomb-add-invalid", X: p.X, Y: p.Y, ToX: bombX, ToY: bombY}
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

				if err = player.send(v); err != nil {
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
	var tileBlocking = isTileBlocking(p.Map, toX, toY)

	if tileBlocking {
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

func (p *Player) canAddBombTo(toX, toY int) bool {
	// valida o tempo
	currentTime := time.Now().UTC()
	lastAddBombTime := p.LastAddBombTime
	diff := currentTime.Sub(lastAddBombTime).Nanoseconds() / int64(time.Millisecond)

	if diff <= p.AddBombDelay {
		debug(fmt.Sprintf("Player cannot add bomb (add bomb delay) - %v, %v, %v", currentTime, lastAddBombTime, diff))
		return false
	}

	// valida o tile
	var idx = toX + toY * maps[p.Map].Layers[0].Width
	var gid = maps[p.Map].Layers[0].Data[idx]

	if gid > 0 {
		debug("Player cannot add bomb (map block)")
		return false
	}

	// valida a posição
	if (toX != p.X) {
		debug("Player cannot add bomb (invalid position - different from player)")
		return false
	} else if (toY != p.Y) {
		debug("Player cannot add bomb (invalid position - different from player)")
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

	player.Map = "001"
	player.CharType = "002"
	player.Direction = 3
	player.MovementDelay = 200 //float64(randomInt(50, 200))
	player.LastMovementTime = time.Now().UTC()
	player.LastPingTime = time.Now().UTC()
	player.LastAddBombTime = time.Now().UTC()
	player.AddBombDelay = 1000
	player.Online = false
	player.X = 0
	player.Y = 0

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

			for _, p := range Players {
				if p.Id == player.Id {
					removePlayer(p)
				} else {
					debug(fmt.Sprintf("Destroy player: %v", player))

					if err = p.send(player.createPlayerRemovedMessage()); err != nil {
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

					addPlayer(player)
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

				player.Online = true

				var tileBlocking = true
				var playerPosX = 0
				var playerPosY = 0

				for tileBlocking {
					playerPosX = randomInt(0, maps[player.Map].Layers[0].Width - 1)
					playerPosY = randomInt(0, maps[player.Map].Layers[0].Height - 1)
					tileBlocking = isTileBlocking(player.Map, playerPosX, playerPosY)

					if !tileBlocking {
						player.X = playerPosX
						player.Y = playerPosY
					}
				}

				if err = player.send(player.createPlayerDataMessage()); err != nil {
					debug(fmt.Sprintf("Error on send command: %v", err))
				}

				debug("Sent")

				// envia a posição do novo player para todos
				debug("Publishing positions...")

				go func() {
					for _, p := range Players {
						if p.Id != player.Id {
							if err = player.send(p.createPlayerAddedMessage()); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}

							if err = p.send(player.createPlayerAddedMessage()); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}
						}
					}
				}()

				debug("Published")
			} else if messageDataType == "bomb-add" {
				// ++++++++++++++++++++++++++++++++++++++++++
				// bomb-add = adiciona uma nova bomba
				// ++++++++++++++++++++++++++++++++++++++++++
				debug("Adding new bomb...")

				var bombX, bombY int

				if value, ok := messageData["x"]; ok {
					bombX = int(value.(float64))
				}

				if value, ok := messageData["y"]; ok {
					bombY = int(value.(float64))
				}

				if (player.canAddBombTo(bombX, bombY)) {
					player.LastAddBombTime = time.Now().UTC()

					bomb := &Bomb{
						Id: uuid.New(),
						X: bombX,
						Y: bombY,
						BombType: "001",
						Direction: 1,
						MovementDelay: 0,
						LastMovementTime: time.Now().UTC(),
						CreatedAt: time.Now().UTC(),
						FireDelay: 2000,
						FireLength: 3,
						Player: player,
					}

					addBomb(bomb)

					go func() {
						for _, p := range Players {
							if err = p.send(player.createBombAddedMessage(bomb)); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}
						}
					}()

					debug("Added and published")
				} else {
					if err = player.send(player.createBombAddInvalidMessage(bombX, bombY)); err != nil {
						debug(fmt.Sprintf("Error on send command: %v", err))
					}
				}
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

	go func() {
		// +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
		// essa rotina observa as bombas e mata os players e blocos
		// +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

		for range tickerBombs.C {
			for _, bomb := range Bombs {
				currentTime := time.Now().UTC()
				bombCreatedAt := bomb.CreatedAt
				diff := currentTime.Sub(bombCreatedAt).Nanoseconds() / int64(time.Millisecond)

				if diff > bomb.FireDelay {
					removeBomb(bomb)

					var explosionPointList = make([]*Point, 0)
					explosionPointList = append(explosionPointList, &Point{X: bomb.X, Y: bomb.Y})

					for x := 0; x < bomb.FireLength; x++ {
						explosionPointList = append(explosionPointList, &Point{X: bomb.X + (x + 1), Y: bomb.Y})
						explosionPointList = append(explosionPointList, &Point{X: bomb.X - (x + 1), Y: bomb.Y})
						explosionPointList = append(explosionPointList, &Point{X: bomb.X, Y: bomb.Y + (x + 1)})
						explosionPointList = append(explosionPointList, &Point{X: bomb.X, Y: bomb.Y - (x + 1)})
					}

					for _, p := range Players {
						collidedWithPlayer := inPointList(p.X, p.Y, explosionPointList)
						var err error

						if collidedWithPlayer {
							p.Online = false

							if err = p.send(p.createBombFiredMessage(bomb)); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}

							if err = p.send(p.createSimpleMessage("dead")); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}

							p.sendToAll(p.createPlayerDeadMessage());
						} else {
							if err = p.send(p.createBombFiredMessage(bomb)); err != nil {
								debug(fmt.Sprintf("Error on send command: %v", err))
							}
						}
					}
				}
			}
		}
	}()

	http.Handle("/ws", websocket.Handler(wsHandler))
	http.Handle("/public", http.FileServer(http.Dir("public")))

	err := http.ListenAndServe(":3030", nil)

	if err != nil {
		debug("Fatal Error: " + err.Error())
		os.Exit(1)
	}
}
