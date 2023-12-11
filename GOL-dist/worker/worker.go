package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/rpc"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
)

var workerNo int
var HaloCh = make(chan []byte)  //used to send and receive halos
var HaloCh2 = make(chan []byte) //used to send and receive halos
var WorldCh = make(chan [][]byte)
var KeyPressCh = make(chan string, 2)
var KillCh = make(chan bool)
var QuitCh = make(chan bool)
var TurnCh2 = make(chan int)
var TurnCh3 = make(chan int)
var Mutex sync.Mutex
var turn int
var AliveController = true

func TickerKeyPress(key string, world [][]byte, turn int) {
	Mutex.Lock()
	if key == "ticker" {
		TurnCh3 <- turn
		WorldCh <- world
	} else if key == "s" {
		TurnCh3 <- turn
		WorldCh <- world
	} else if key == "k" {
		TurnCh3 <- turn
		WorldCh <- world
		KillCh <- true
	} else if key == "q" {
		TurnCh3 <- turn
		WorldCh <- world
		QuitCh <- true
	} else if key == "p" {
		TurnCh3 <- turn
		WorldCh <- world
	out:
		for {
			select {
			case unpause := <-KeyPressCh:
				if unpause == "p" {
					<-TurnCh2
					TurnCh3 <- turn
					WorldCh <- world
					break out
				}
			}
		}
	}
	Mutex.Unlock()
}

func CallHandler(world [][]byte, odd bool, p stubs.Param, prev string, next string) ([][]byte, int) {
	p.ImageHeight = len(world)
	var key string
	keyTurn := math.MinInt
out:
	for t := 0; t < p.Turns; t++ {
		select {
		case key = <-KeyPressCh:
			keyTurn = <-TurnCh2
			t--
		case <-QuitCh:
			AliveController = false
			WorldCh <- world
			TurnCh2 <- turn
			break out
		default:
			if t == keyTurn {
				go TickerKeyPress(key, world, turn)
			}
			Mutex.Lock()
			//Create a world that can store the halos
			hWorldLength := p.ImageHeight + 2
			haloWorld := make([][]byte, hWorldLength)
			for i := range haloWorld {
				if i == 0 || i == hWorldLength-1 {
					haloWorld[i] = make([]byte, p.ImageWidth) //add empty for halos on 0th and last index
				} else {
					haloWorld[i] = world[i-1]
				}
			}
			if odd {
				callNext, _ := rpc.Dial("tcp", next)
				go CallAdjacentWorker(callNext, world[p.ImageHeight-1], t+1, odd) //sends last halo to next one
				defer callNext.Close()

				halo := <-HaloCh //receives the first halo of next one
				haloWorld[hWorldLength-1] = halo
				go CalculateNextState(p, haloWorld, odd)
				HaloCh <- world[0]
			} else {
				HaloCh <- world[0]
				halo := <-HaloCh //receives the last halo of the previous one
				haloWorld[0] = halo
				go CalculateNextState(p, haloWorld, odd)

				callPrev, _ := rpc.Dial("tcp", prev)
				go CallAdjacentWorker(callPrev, world[p.ImageHeight-1], t+1, odd) //sends first halo to next one and receives its first halo in go
				defer callPrev.Close()
			}
			world = <-WorldCh
			turn = t + 1
			Mutex.Unlock()
		}
	}
	return world, turn
}
func CallAdjacentWorker(client *rpc.Client, halo []byte, turn int, odd bool) {
	request := stubs.Halo{Halo: halo, Turn: turn, Odd: odd}
	response := new(stubs.Halo)
	err := client.Call(stubs.WorkerCall, request, response)
	if err != nil {
		fmt.Println(err)
		return
	}
	if odd {
		HaloCh <- response.Halo
	} else {
		HaloCh2 <- response.Halo
	}
}

type GolOperations struct{}

func (s *GolOperations) Execute(req stubs.BrokerRequest, res *stubs.Response) (err error) {
	if AliveController {
		workerNo = req.WorkerNo
		res.World, res.Turn = CallHandler(req.Slice, req.WorkerNo%2 != 0, req.P, req.PrevWorker, req.NextWorker)
	} else {
		res.World = <-WorldCh
		res.Turn = <-TurnCh2
	}
	res.WorkerNo = req.WorkerNo
	return
}

func (s *GolOperations) KeyTicker(key stubs.KeyTickerInput, res *int) (err error) {
	KeyPressCh <- key.Key
	*res = turn
	return
}
func (s *GolOperations) KeyTickerOutput(keyTurn int, res *stubs.Response) (err error) {
	TurnCh2 <- keyTurn
	res.Turn = <-TurnCh3
	res.World = <-WorldCh
	res.WorkerNo = workerNo
	return
}

func (s *GolOperations) WorkerCom(halo stubs.Halo, res *stubs.Halo) (err error) {
	if !halo.Odd {
		res.Halo = <-HaloCh
		HaloCh2 <- halo.Halo
	} else {
		res.Halo = <-HaloCh
		HaloCh <- halo.Halo
	}
	res.Turn = 0
	return
}

func CalculateNextState(p stubs.Param, world [][]byte, odd bool) {
	nextSlice := make([][]byte, p.ImageHeight)
	for i := range nextSlice {
		nextSlice[i] = make([]byte, p.ImageWidth)
	}

	var startY int
	var endY int
	var iterate int
	if odd {
		startY = len(world) - 2 //if odd worker then starts from the bottom ignoring halos
		endY = 1
		iterate = -1 //minus one
	} else {
		startY = 1 //if even starts from the top ignoring halos
		endY = len(world) - 2
		iterate = 1 //add one
	}
	for i := startY; i != endY+iterate; i += iterate {
		if odd && i == endY { //before calculating line one, add first halo
			firstHalo := <-HaloCh2
			world[0] = firstHalo
		} else if !odd && i == endY { //before calculating line last, add last halo
			lastHalo := <-HaloCh2
			world[endY+1] = lastHalo
		}
		for j := 0; j < p.ImageWidth; j++ {
			liveNeighbours := 0
			for c := -1; c <= 1; c++ {
				for r := -1; r <= 1; r++ {
					if c == 0 && r == 0 {
						continue //skips itself
					}
					ni, nj := i+c, j+r //we don't need to mod because we never go out of bounds
					if nj == -1 {
						nj = len(world[i]) - 1
					} else if nj == len(world[i]) {
						nj = 0
					}
					if world[ni][nj] == 255 {
						liveNeighbours++
					}
				}
			}
			if liveNeighbours < 2 || liveNeighbours > 3 { //improved cell state logic
				nextSlice[i-1][j] = 0 //we don't need indexes because we don't pass the whole world
			} else if liveNeighbours == 3 {
				nextSlice[i-1][j] = 255
			} else {
				nextSlice[i-1][j] = world[i][j]
			}
		}
	}
	WorldCh <- nextSlice
}

func main() {
	pAddr := flag.String("port", "8040", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	rpc.Register(&GolOperations{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	go func() {
		<-KillCh
		listener.Close()
	}()
	rpc.Accept(listener)

}
