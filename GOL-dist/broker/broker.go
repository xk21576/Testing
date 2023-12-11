package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
)

type WorkerWorld struct {
	WorkerNo int
	World    [][]byte
	Turn     int
}

/* NON PARALLEL VERSION
var WorldCh = make(chan [][]byte)
var TurnCh = make(chan int)
func ExecuteTurns(p stubs.Param, world [][]byte) ([][]byte, int) {
	turn := 0
	for t := 0; t < p.Turns; t++ {
		select {
		case key := <-KeyPressCh:
			if key == "ticker" {
				WorldCh <- world
				TurnCh <- turn
			} else if key == "s" {
				WorldCh <- world
				TurnCh <- turn
			} else if key == "k" {
				return world, turn
			} else if key == "p" {
				fmt.Println("Paused")
			out:
				for {
					select {
					case unpause := <-KeyPressCh:
						if unpause == "p" {
							fmt.Println("Unpaused")
							break out
						}
					}
				}
			}
			t--
		default:
			world = serverCalculateNextState(p, world)
			turn = t + 1
		}
	}
	return world, turn
}*/

// ------------------------------------------------------------------------
func kill() {
	select {
	case <-time.After(2 * time.Second):
		os.Exit(1)
	}
}
func keyPressesAndTicker(keyPress string) ([][]byte, int) {
	workerAddr := []string{
		"54.221.14.115:8040", "54.221.14.115:8041",
	}
	var TurnCh = make(chan int)
	var WorldCh = make(chan [][]byte)
	var Workers = make(chan WorkerWorld)
	var turn int
	var completeWorld [][]byte

	for _, ip := range workerAddr {
		server := ip
		client, _ := rpc.Dial("tcp", server)
		go SendKeyPress(client, keyPress, TurnCh)
		defer client.Close()
	}

	for range workerAddr {
		t := <-TurnCh
		if t > turn {
			turn = t
		}
	}
	turn = turn + 1

	for _, ip := range workerAddr {
		server := ip
		client, _ := rpc.Dial("tcp", server)
		go GetOutput(client, turn, Workers)
		defer client.Close()
	}

	go orderWorld(len(workerAddr), Workers, WorldCh, TurnCh)
	completeWorld = <-WorldCh
	turn = <-TurnCh
	if keyPress == "k" {
		kill()
	}
	return completeWorld, turn
}

func GetOutput(client *rpc.Client, turn int, Workers chan WorkerWorld) {
	request := turn
	response := new(stubs.Response)
	err := client.Call(stubs.KeyTickerOutput, request, response)
	if err != nil {
		fmt.Println(err)
	}
	workerW := WorkerWorld{
		response.WorkerNo,
		response.World,
		response.Turn,
	}
	Workers <- workerW
}

func SendKeyPress(client *rpc.Client, keyPress string, TurnCh chan int) {
	request := stubs.KeyTickerInput{Key: keyPress}
	response := new(int)
	err := client.Call(stubs.KeyAndTickerHandler, request, response)
	if err != nil {
		fmt.Println(err)
	}
	TurnCh <- *response
}

func ExecuteTurns(p stubs.Param, world [][]byte) ([][]byte, int) {
	workerAddr := []string{
		"54.221.14.115:8040", "54.221.14.115:8041",
	}
	var Workers = make(chan WorkerWorld)
	var TurnCh = make(chan int)
	var WorldCh = make(chan [][]byte)

	for i, ip := range workerAddr {
		var previousWorker string
		var nextWorker string
		if i == 0 {
			previousWorker = workerAddr[len(workerAddr)-1]
			nextWorker = workerAddr[i+1]
		} else if i == len(workerAddr)-1 {
			previousWorker = workerAddr[i-1]
			nextWorker = workerAddr[0]
		} else {
			previousWorker = workerAddr[i-1]
			nextWorker = workerAddr[i+1]
		}
		startY := i * p.ImageHeight / len(workerAddr)
		endY := (i + 1) * p.ImageHeight / len(workerAddr)

		slice := make([][]byte, endY-startY)
		for n := range slice {
			slice[n] = world[n+startY]
		}
		server := ip
		client, _ := rpc.Dial("tcp", server)
		go CallWorkers(client, slice, nextWorker, previousWorker, p, i, Workers) //odd
		defer client.Close()
	}
	go orderWorld(len(workerAddr), Workers, WorldCh, TurnCh)
	completeWorld := <-WorldCh
	turn := <-TurnCh
	return completeWorld, turn
}

func orderWorld(workerCount int, Workers chan WorkerWorld, WorldCh chan [][]byte, TurnCh chan int) {
	workers := make([]WorkerWorld, workerCount)

	for i := 0; i < workerCount; i++ {
		worker := <-Workers
		workers[worker.WorkerNo] = worker
	}
	var completeWorld [][]byte
	for _, worker := range workers {
		completeWorld = append(completeWorld, worker.World...)

	}
	WorldCh <- completeWorld
	TurnCh <- workers[0].Turn

}

func CallWorkers(client *rpc.Client, slice [][]byte, nextWorker, previousWorker string, p stubs.Param, workerNo int, Workers chan WorkerWorld) {
	request := stubs.BrokerRequest{
		Slice:      slice,
		P:          p,
		WorkerNo:   workerNo,
		PrevWorker: previousWorker,
		NextWorker: nextWorker,
	}

	response := new(stubs.Response)
	err := client.Call(stubs.TurnsHandler, request, response)
	if err != nil {
		fmt.Println(err)
	}

	workerW := WorkerWorld{
		response.WorkerNo,
		response.World,
		response.Turn,
	}
	Workers <- workerW
}

func (s *GolOperations) KeyTicker(keyPress stubs.KeyTickerInput, res *stubs.Response) (err error) {
	res.World, res.Turn = keyPressesAndTicker(keyPress.Key)
	return
}

// ----------------------
type GolOperations struct{}

func (s *GolOperations) Execute(req stubs.Request, res *stubs.Response) (err error) {
	//the execute method will call the execute turns method for the amount of turns in p.turns
	res.World, res.Turn = ExecuteTurns(req.P, req.World)
	return
}

/*
func (s *GolOperations) KeyTicker(keyPress stubs.KeyTickerInput, res *stubs.Response) (err error) {
	KeyPressCh <- keyPress.Key
	if keyPress.Key == "s" || keyPress.Key == "ticker" {
		res.World = <-WorldCh
		res.Turn = <-TurnCh
		fmt.Println("ticker keyticker", len(res.World), res.Turn)
	}
	return
}
*/

func CalculateNextState(p stubs.Param, world, nextSlice [][]byte) [][]byte {
	for i := 0; i < p.ImageHeight; i++ { //Only go between the given indexes
		for j := 0; j < p.ImageWidth; j++ {
			liveNeighbours := 0
			for c := -1; c <= 1; c++ { //added the iteration we talked about
				for r := -1; r <= 1; r++ {
					if c == 0 && r == 0 {
						continue //skips itself
					}
					ni, nj := (i+c+p.ImageHeight)%p.ImageHeight, (j+r+p.ImageWidth)%p.ImageWidth
					if world[ni][nj] == 255 {
						liveNeighbours++
					}
				}
			}
			if liveNeighbours < 2 || liveNeighbours > 3 { //improved cell state logic
				if world[i][j] == 255 {
				}
				nextSlice[i][j] = 0 //We minus the startY when indexing because slice has a smaller height
			} else if liveNeighbours == 3 {
				if world[i][j] == 0 {
				}
				nextSlice[i][j] = 255
			} else {
				nextSlice[i][j] = world[i][j]
			}
		}
	}
	return nextSlice
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	rpc.Register(&GolOperations{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	rpc.Accept(listener)
}
