package main

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/KyberNetwork/deribit-api/pkg/multicast"
	ws "github.com/KyberNetwork/deribit-api/pkg/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const ()

var (
	debug              = flag.Bool("debug", true, "Enable debug logs")
	wsEndpoint         = flag.String("websocket", "ws://193.58.254.1:8022/ws/api/v2", "Websocket API endpoint")
	apiKey             = flag.String("api-key", "", "API client ID")
	secretKey          = flag.String("secret-key", "", "API secret key")
	ifname             = flag.String("ifname", "bond-colocation", "Interface name to listen for multicast events")
	addrs              = flag.String("addrs", "239.111.111.2:6100,239.111.111.2:6100,239.111.111.3:6100", "UDP addresses to listen for multicast events")
	gatherDataDuration = flag.Duration("gather-data-duration", 3*time.Minute, "gather data duration")
	storagePath        = flag.String("storage-path", "libs/data/", "API secret key")
	log                *zap.SugaredLogger
)

func setupLogger(debug bool) *zap.SugaredLogger {
	pConf := zap.NewProductionEncoderConfig()
	pConf.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewConsoleEncoder(pConf)
	level := zap.NewAtomicLevelAt(zap.InfoLevel)
	if debug {
		level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	l := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level), zap.AddCaller())
	zap.ReplaceGlobals(l)
	return zap.S()
}

func saveData(data interface{}, filename string) error {
	f, err := os.Create(*storagePath + filename)
	if err != nil {
		log.Error("Fail to create file", "filename", filename, "error", err)
		return err
	}

	err = json.NewEncoder(f).Encode(data)
	if err != nil {
		log.Error("Fail to write data to file", "error", err)
		return err
	}

	return nil
}

// listen to multicast orderbook events
func listenToOrderbookEvent(m *multicast.Client) {
	orderbookChannels := []string{
		"book.BTC-PERPETUAL",
		"book.BTC-1AUG22-29000-P",
	}
	data := make([]models.OrderBookRawNotification, 0)
	listener := func(e *models.OrderBookRawNotification) {
		data = append(data, *e)
	}
	for _, channel := range orderbookChannels {
		m.On(channel, listener)

	}
	time.Sleep(*gatherDataDuration)
	for _, channel := range orderbookChannels {
		m.Off(channel, listener)
	}
	saveData(data, "orderbook.json")
}

// listen to multicast trades events
func listenToTradesEvent(m *multicast.Client) {
	tradesChannels := []string{
		"trade.option.BTC",
		"trade.future.BTC",
	}
	data := make([]models.TradesNotification, 0)
	listener := func(e *models.TradesNotification) {
		data = append(data, *e)
	}
	for _, channel := range tradesChannels {
		m.On(channel, listener)

	}
	time.Sleep(*gatherDataDuration)
	for _, channel := range tradesChannels {
		m.Off(channel, listener)
	}

	saveData(data, "trades.json")
}

// listen to multicast ticker events
func listenToTickerEvent(m *multicast.Client) {
	tickerChannels := []string{
		"ticker.BTC-PERPETUAL",
		"ticker.BTC-1AUG22-29000-P",
	}
	data := make([]models.TickerNotification, 0)
	listener := func(e *models.TickerNotification) {
		data = append(data, *e)
	}
	for _, channel := range tickerChannels {
		m.On(channel, listener)

	}
	time.Sleep(*gatherDataDuration)
	for _, channel := range tickerChannels {
		m.Off(channel, listener)
	}
	saveData(data, "ticker.json")
}

func main() {
	wsConfig := &ws.Configuration{
		Addr:          *wsEndpoint,
		ApiKey:        *apiKey,
		SecretKey:     *secretKey,
		AutoReconnect: true,
		DebugMode:     true,
	}
	log = setupLogger(*debug)

	wsClient := ws.New(log, wsConfig)
	udpAddrs := strings.Split(*addrs, ",")
	multicastClient, err := multicast.NewClient(*ifname, udpAddrs, wsClient, []string{"BTC"})
	if err != nil {
		log.Errorw("failed to initiate multicast client", "ifname", ifname, "addrs", addrs)
		panic(err)
	}

	go listenToOrderbookEvent(multicastClient)
	go listenToTradesEvent(multicastClient)
	go listenToTickerEvent(multicastClient)

	time.Sleep(*gatherDataDuration + 3*time.Second)
}
