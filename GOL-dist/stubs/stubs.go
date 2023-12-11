package stubs

var TurnsHandler = "GolOperations.Execute"
var KeyAndTickerHandler = "GolOperations.KeyTicker"
var WorkerCall = "GolOperations.WorkerCom"
var KeyTickerOutput = "GolOperations.KeyTickerOutput"

type Param struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}
type Request struct {
	World     [][]byte
	P         Param
	OddWorker bool
}

type BrokerRequest struct {
	Slice      [][]byte
	P          Param
	WorkerNo   int
	PrevWorker string
	NextWorker string
}

type Halo struct {
	Halo []byte
	Turn int
	Odd  bool
}

type KeyTickerInput struct {
	Key string
}

type Response struct {
	World    [][]byte
	Turn     int
	WorkerNo int
}
