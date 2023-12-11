package gol

import (
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

func calculateAliveCells(world [][]byte) []util.Cell {
	var aliveCells []util.Cell
	for y, row := range world {
		for x, value := range row {
			if value == 255 {
				aliveCells = append(aliveCells, util.Cell{x, y})
			}
		}
	}
	return aliveCells
}

func makeCall(client *rpc.Client, world [][]byte, p stubs.Param, c distributorChannels, fileName string) {
	request := stubs.Request{World: world, P: p}
	response := new(stubs.Response)
	err := client.Call(stubs.TurnsHandler, request, response)
	if err != nil {
		fmt.Println(err)
		return
	}
	output(c, fileName, response.Turn, response.World, true)
}

func keyPressTicker(keyPresses <-chan rune, c distributorChannels, endTicking *bool, fileName string) {
	var command string
	paused := false
	reportTime := time.NewTicker(2 * time.Second)
	for {
		if paused {
			reportTime.Stop()
		} else {
			reportTime = time.NewTicker(2 * time.Second)
		}
		select {
		case <-reportTime.C:
			if *endTicking {
				return
			}
			command = "ticker"
			server := "127.0.0.1:8030"
			client, _ := rpc.Dial("tcp", server)
			keyPressTickerCall(client, command, c, fileName, paused)
			defer client.Close()
		case t := <-keyPresses:
			command = string(t)
			if command == "p" && !paused {
				paused = true
			} else if command == "p" && paused {
				paused = false
			}
			server := "127.0.0.1:8030"
			client, _ := rpc.Dial("tcp", server)
			keyPressTickerCall(client, command, c, fileName, paused)
			defer client.Close()
		}
	}
}

func keyPressTickerCall(client *rpc.Client, command string, c distributorChannels, fileName string, paused bool) {
	request := stubs.KeyTickerInput{Key: command}
	response := new(stubs.Response)
	err := client.Call(stubs.KeyAndTickerHandler, request, response)
	if err != nil {
		fmt.Println(err)
		return
	}
	if command == "q" {
		os.Exit(1)
	} else if command == "k" {
		output(c, fileName, response.Turn, response.World, true)
	} else if command == "s" {
		output(c, fileName, response.Turn, response.World, false)
	} else if command == "ticker" {
		c.events <- AliveCellsCount{CompletedTurns: response.Turn, CellsCount: len(calculateAliveCells(response.World))}
	} else if command == "p" && !paused {
		fmt.Println("Continuing")
	} else if command == "p" && paused {
		fmt.Println("Paused")
	}
}

func output(c distributorChannels, fileName string, turn int, world [][]byte, final bool) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fileName + "x" + strconv.Itoa(turn)
	for _, row := range world {
		for _, cell := range row {
			c.ioOutput <- cell
		}
	}
	if final {
		c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: calculateAliveCells(world)}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	// TODO: Create a 2D slice to store the world.
	c.ioCommand <- ioInput
	fileName := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth)
	c.ioFilename <- fileName

	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
		for n := 0; n < p.ImageWidth; n++ {
			input := <-c.ioInput
			world[i][n] = input
		}
	}
	pa := stubs.Param{
		Turns:       p.Turns,
		Threads:     p.Threads,
		ImageWidth:  p.ImageWidth,
		ImageHeight: p.ImageHeight,
	}
	endTicking := false
	go keyPressTicker(keyPresses, c, &endTicking, fileName)

	server := "127.0.0.1:8030"
	client, _ := rpc.Dial("tcp", server)
	makeCall(client, world, pa, c, fileName)
	defer client.Close()
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	turn := p.Turns
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	endTicking = true
	close(c.events)
}
