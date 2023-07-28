package multicast

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/KyberNetwork/deribit-api/pkg/multicast/sbe"
	"github.com/chuckpreslar/emission"
	"go.uber.org/zap"
	"golang.org/x/net/ipv4"
)

const (
	maxPacketSize     = 1500
	defaultDataChSize = 1000

	RestartEventChannel = "multicast.restart"
)

type EventType int

// Defines constants for types of events.
const (
	EventTypeInstrument EventType = iota
	EventTypeOrderBook
	EventTypeTrades
	EventTypeTicker
	EventTypeSnapshot
)

// Event represents a Deribit multicast events.
type Event struct {
	Type EventType
	Data interface{}
}

type InstrumentsGetter interface {
	GetInstruments(ctx context.Context, params *models.GetInstrumentsParams) ([]models.Instrument, error)
}

// Client represents a client for Deribit multicast.
type Client struct {
	log               *zap.SugaredLogger
	inf               *net.Interface
	addrs             []string
	connMap           map[int]*ipv4.PacketConn // port: packetConnection
	instrumentsGetter InstrumentsGetter

	supportCurrencies []string
	instrumentsMap    map[uint32]models.Instrument
	emitter           *emission.Emitter
}

// NewClient creates a new Client instance.
func NewClient(
	ifname string,
	addrs []string,
	instrumentsGetter InstrumentsGetter,
	currencies []string,
) (client *Client, err error) {
	log := zap.S()

	var inf *net.Interface
	if ifname != "" {
		inf, err = net.InterfaceByName(ifname)
		if err != nil {
			log.Errorw("failed to create net interfaces by name", "err", err, "ifname", ifname)
			return nil, err
		}
	}

	client = &Client{
		log:               log,
		inf:               inf,
		addrs:             addrs,
		instrumentsGetter: instrumentsGetter,

		supportCurrencies: currencies,
		instrumentsMap:    make(map[uint32]models.Instrument),
		emitter:           emission.NewEmitter(),
	}

	return client, nil
}

// buildInstrumentsMapping builds a mapping to map instrument id to instrument.
func (c *Client) buildInstrumentsMapping() error {
	// need to update this field via instruments_update event
	instruments, err := getAllInstrument(c.instrumentsGetter, c.supportCurrencies)
	if err != nil {
		c.log.Errorw("failed to get all instruments", "err", err)
		return err
	}
	for _, instrument := range instruments {
		c.instrumentsMap[instrument.InstrumentID] = instrument
	}
	return nil
}

// getAllInstrument returns a list of all instruments by currencies
func getAllInstrument(
	instrumentsGetter InstrumentsGetter, currencies []string,
) ([]models.Instrument, error) {
	result := make([]models.Instrument, 0)
	for _, currency := range currencies {
		ins, err := instrumentsGetter.GetInstruments(
			context.Background(),
			&models.GetInstrumentsParams{
				Currency: currency,
			})
		if err != nil {
			return nil, err
		}
		result = append(result, ins...)
	}
	return result, nil
}

// decodeEvents decodes a UDP package into a list of events.
func (c *Client) decodeEvents(
	marshaller *sbe.SbeGoMarshaller, reader io.Reader,
	bookChangesMap map[string][]sbe.BookChangesList,
	snapshotLevelsMap map[string][]sbe.SnapshotLevelsList,
) (events []Event, err error) {
	// Decode Sbe messages
	for {
		// Decode message header.
		var header sbe.MessageHeader
		err = header.Decode(marshaller, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			return
		}

		event, err := c.decodeEvent(marshaller, reader, header, bookChangesMap, snapshotLevelsMap)
		if err != nil {
			if errors.Is(err, ErrEventWithoutIsLast) {
				continue
			}
			if errors.Is(err, ErrUnsupportedTemplateID) {
				c.log.Debugw("Ignore unsupported event", "error", err)
				continue
			}
			return nil, err
		}
		events = append(events, event)
	}
}

func (c *Client) decodeEvent(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	header sbe.MessageHeader,
	bookChangesMap map[string][]sbe.BookChangesList,
	snapshotLevelsMap map[string][]sbe.SnapshotLevelsList,
) (Event, error) {
	switch header.TemplateId {
	case 1000:
		return c.decodeInstrumentEvent(marshaller, reader, header)
	case 1001:
		return c.decodeOrderBookEvent(marshaller, reader, header, bookChangesMap)
	case 1002:
		return c.decodeTradesEvent(marshaller, reader, header)
	case 1003:
		return c.decodeTickerEvent(marshaller, reader, header)
	case 1004:
		return c.decodeSnapshotEvent(marshaller, reader, header, snapshotLevelsMap)
	case 1010:
		return c.decodeInstrumentV2Event(marshaller, reader, header)
	default:
		err := decodeUnsupportedEvent(marshaller, reader, header)
		if err != nil {
			return Event{}, err
		}
		return Event{}, fmt.Errorf("%w, templateId: %d", ErrUnsupportedTemplateID, header.TemplateId)
	}
}

func (c *Client) decodeInstrumentEvent(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	header sbe.MessageHeader,
) (Event, error) {
	var ins sbe.Instrument
	err := ins.Decode(marshaller, reader, header.BlockLength, true)
	if err != nil {
		c.log.Errorw("failed to decode instrument event", "err", err)
		return Event{}, err
	}

	instrument := models.Instrument{
		TickSize:             ins.TickSize,
		TakerCommission:      ins.TakerCommission,
		SettlementPeriod:     ins.SettlementPeriod.String(),
		QuoteCurrency:        getCurrencyFromBytesArray(ins.QuoteCurrency),
		MinTradeAmount:       ins.MinTradeAmount,
		MakerCommission:      ins.MakerCommission,
		Leverage:             int(ins.MaxLeverage),
		Kind:                 ins.Kind.String(),
		IsActive:             ins.IsActive(),
		InstrumentID:         ins.InstrumentId,
		InstrumentName:       string(ins.InstrumentName),
		ExpirationTimestamp:  ins.ExpirationTimestampMs,
		CreationTimestamp:    ins.CreationTimestampMs,
		ContractSize:         ins.ContractSize,
		BaseCurrency:         getCurrencyFromBytesArray(ins.BaseCurrency),
		BlockTradeCommission: ins.BlockTradeCommission,
		OptionType:           ins.OptionType.String(),
		Strike:               ins.StrikePrice,
	}

	return Event{
		Type: EventTypeInstrument,
		Data: instrument,
	}, nil
}

func (c *Client) decodeInstrumentV2Event(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	header sbe.MessageHeader,
) (Event, error) {
	var ins sbe.InstrumentV2
	err := ins.Decode(marshaller, reader, header.BlockLength, true)
	if err != nil {
		c.log.Errorw("failed to decode instrument event", "err", err)
		return Event{}, err
	}

	tickSizeSteps := make([]models.TickSizeStep, 0, len(ins.TickStepsList))
	for _, step := range ins.TickStepsList {
		tickSizeSteps = append(tickSizeSteps, models.TickSizeStep{
			AbovePrice: step.AbovePrice,
			TickSize:   step.TickSize,
		})
	}

	instrument := models.Instrument{
		TickSize:             ins.TickSize,
		TakerCommission:      ins.TakerCommission,
		SettlementPeriod:     ins.SettlementPeriod.String(),
		QuoteCurrency:        getCurrencyFromBytesArray(ins.QuoteCurrency),
		MinTradeAmount:       ins.MinTradeAmount,
		MakerCommission:      ins.MakerCommission,
		Leverage:             int(ins.MaxLeverage),
		Kind:                 ins.Kind.String(),
		IsActive:             ins.IsActive(),
		InstrumentID:         ins.InstrumentId,
		InstrumentName:       string(ins.InstrumentName),
		ExpirationTimestamp:  ins.ExpirationTimestampMs,
		CreationTimestamp:    ins.CreationTimestampMs,
		ContractSize:         ins.ContractSize,
		BaseCurrency:         getCurrencyFromBytesArray(ins.BaseCurrency),
		BlockTradeCommission: ins.BlockTradeCommission,
		OptionType:           ins.OptionType.String(),
		Strike:               ins.StrikePrice,
		TickSizeSteps:        tickSizeSteps,
	}

	return Event{
		Type: EventTypeInstrument,
		Data: instrument,
	}, nil
}

//nolint:dupl
func (c *Client) decodeOrderBookEvent(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	header sbe.MessageHeader,
	bookChangesMap map[string][]sbe.BookChangesList,
) (Event, error) {
	var book sbe.Book
	err := book.Decode(marshaller, reader, header.BlockLength, true)
	if err != nil {
		c.log.Errorw("failed to decode orderbook event", "err", err)
		return Event{}, err
	}

	instrumentName := c.getInstrument(book.InstrumentId).InstrumentName
	key := instrumentName + strconv.FormatUint(book.ChangeId, 10)

	if book.IsLast == sbe.YesNo.No {
		c.log.Infow("Received multicast orderbook with isLast=No",
			"instrument", instrumentName,
			"multicast_orderbook", book,
		)
		changeList, ok := bookChangesMap[key]
		if ok {
			bookChangesMap[key] = append(changeList, book.ChangesList...)
		} else {
			bookChangesMap[key] = book.ChangesList
		}
		return Event{}, fmt.Errorf("orderbook: %w", ErrEventWithoutIsLast)
	}

	return parseSbeBookToEvent(instrumentName, book, bookChangesMap, key), nil
}

func parseSbeBookToEvent(
	instrumentName string,
	book sbe.Book,
	bookChangesMap map[string][]sbe.BookChangesList,
	key string,
) Event {
	event := models.OrderBookRawNotification{
		Timestamp:      int64(book.TimestampMs),
		InstrumentName: instrumentName,
		PrevChangeID:   int64(book.PrevChangeId),
		ChangeID:       int64(book.ChangeId),
	}

	if bookChanges, ok := bookChangesMap[key]; ok {
		book.ChangesList = append(bookChanges, book.ChangesList...)
		delete(bookChangesMap, key)
	}

	for _, bookChange := range book.ChangesList {
		item := models.OrderBookNotificationItem{
			Action: bookChange.Change.String(),
			Price:  bookChange.Price,
			Amount: bookChange.Amount,
		}

		if bookChange.Side == sbe.BookSide.Ask {
			event.Asks = append(event.Asks, item)
		} else if bookChange.Side == sbe.BookSide.Bid {
			event.Bids = append(event.Bids, item)
		}
	}
	return Event{
		Type: EventTypeOrderBook,
		Data: event,
	}
}

func (c *Client) decodeTradesEvent(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	header sbe.MessageHeader,
) (Event, error) {
	var trades sbe.Trades
	err := trades.Decode(marshaller, reader, header.BlockLength, true)
	if err != nil {
		c.log.Errorw("failed to decode trades event", "err", err)
		return Event{}, err
	}

	ins := c.getInstrument(trades.InstrumentId)

	tradesEvent := make(models.TradesNotification, len(trades.TradesList))
	for i, trade := range trades.TradesList {
		tradesEvent[i] = models.Trade{
			Amount:         trade.Amount,
			BlockTradeID:   strconv.FormatUint(trade.BlockTradeId, 10),
			Direction:      trade.Direction.String(),
			IndexPrice:     trade.IndexPrice,
			InstrumentName: ins.InstrumentName,
			InstrumentKind: ins.Kind,
			IV:             trade.Iv,
			Liquidation:    trade.Liquidation.String(),
			MarkPrice:      trade.MarkPrice,
			Price:          trade.Price,
			TickDirection:  int(trade.TickDirection),
			Timestamp:      trade.TimestampMs,
			TradeID:        strconv.FormatUint(trade.TradeId, 10),
			TradeSeq:       trade.TradeSeq,
		}
	}

	return Event{
		Type: EventTypeTrades,
		Data: tradesEvent,
	}, nil
}

func (c *Client) decodeTickerEvent(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	header sbe.MessageHeader,
) (Event, error) {
	var ticker sbe.Ticker
	err := ticker.Decode(marshaller, reader, header.BlockLength, true)
	if err != nil {
		c.log.Errorw("failed to decode ticker event", "err", err)
		return Event{}, err
	}

	instrumentName := c.getInstrument(ticker.InstrumentId).InstrumentName

	event := models.TickerNotification{
		Timestamp:       ticker.TimestampMs,
		Stats:           models.Stats{},
		State:           ticker.InstrumentState.String(),
		SettlementPrice: ticker.SettlementPrice,
		OpenInterest:    ticker.OpenInterest,
		MinPrice:        ticker.MinSellPrice,
		MaxPrice:        ticker.MaxBuyPrice,
		MarkPrice:       ticker.MarkPrice,
		LastPrice:       ticker.LastPrice,
		InstrumentName:  instrumentName,
		IndexPrice:      ticker.IndexPrice,
		Funding8H:       ticker.Funding8h,
		CurrentFunding:  ticker.CurrentFunding,
		BestBidPrice:    &ticker.BestBidPrice,
		BestBidAmount:   ticker.BestBidAmount,
		BestAskPrice:    &ticker.BestAskPrice,
		BestAskAmount:   ticker.BestAskAmount,
	}

	return Event{
		Type: EventTypeTicker,
		Data: event,
	}, nil
}

//nolint:dupl
func (c *Client) decodeSnapshotEvent(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	header sbe.MessageHeader,
	snapshotLevelsMap map[string][]sbe.SnapshotLevelsList,
) (Event, error) {
	var snapshot sbe.Snapshot
	err := snapshot.Decode(marshaller, reader, header.BlockLength, true)
	if err != nil {
		c.log.Errorw("failed to decode snapshot event", "err", err)
		return Event{}, err
	}

	instrumentName := c.getInstrument(snapshot.InstrumentId).InstrumentName
	key := instrumentName + strconv.FormatUint(snapshot.ChangeId, 10)

	if snapshot.IsLastInBook == sbe.YesNo.No {
		c.log.Infow("Received multicast snapshot with IsLastInBook=No",
			"instrument", instrumentName,
			"snapshot", snapshot,
		)
		levelsList, ok := snapshotLevelsMap[key]
		if ok {
			snapshotLevelsMap[key] = append(levelsList, snapshot.LevelsList...)
		} else {
			snapshotLevelsMap[key] = snapshot.LevelsList
		}
		return Event{}, fmt.Errorf("snapshot: %w", ErrEventWithoutIsLast)
	}

	return parseSbeSnapshotToEvent(instrumentName, snapshot, snapshotLevelsMap, key), nil
}

func discardVars(_m *sbe.SbeGoMarshaller, r io.Reader, numVars uint16) error {
	for i := 0; i < int(numVars); i++ {
		var length uint8
		if err := _m.ReadUint8(r, &length); err != nil {
			return err
		}

		_, _ = io.CopyN(ioutil.Discard, r, int64(length))
	}

	return nil
}

func discardGroup(_m *sbe.SbeGoMarshaller, r io.Reader) error {
	// Read group header.
	var blockLength uint16
	if err := _m.ReadUint16(r, &blockLength); err != nil {
		return err
	}

	var numInGroups uint16
	if err := _m.ReadUint16(r, &numInGroups); err != nil {
		return err
	}

	var numGroups uint16
	if err := _m.ReadUint16(r, &numGroups); err != nil {
		return err
	}

	var numVars uint16
	if err := _m.ReadUint16(r, &numVars); err != nil {
		return err
	}

	// Discard group's fixed length data.
	_, _ = io.CopyN(ioutil.Discard, r, int64(numInGroups*blockLength))

	// Discard sub-groups.
	if numGroups > 0 {
		if err := discardGroups(_m, r, numGroups); err != nil {
			return err
		}
	}

	// Discard variable fields.
	if numVars > 0 {
		if err := discardVars(_m, r, numVars); err != nil {
			return err
		}
	}

	return nil
}

func discardGroups(_m *sbe.SbeGoMarshaller, r io.Reader, numGroups uint16) error {
	for i := uint16(0); i < numGroups; i++ {
		err := discardGroup(_m, r)
		if err != nil {
			return err
		}
	}

	return nil
}

func decodeUnsupportedEvent(
	_m *sbe.SbeGoMarshaller,
	r io.Reader,
	header sbe.MessageHeader,
) error {
	// Discard bytes from fixed fields.
	_, _ = io.CopyN(ioutil.Discard, r, int64(header.BlockLength))

	// Decode and discard groups.
	if header.NumGroups > 0 {
		err := discardGroups(_m, r, header.NumGroups)
		if err != nil {
			return err
		}
	}

	// Decode and discard variable length fields.
	if header.NumVarDataFields > 0 {
		err := discardVars(_m, r, header.NumVarDataFields)
		if err != nil {
			return err
		}
	}

	return nil
}

func parseSbeSnapshotToEvent(
	instrumentName string,
	snapshot sbe.Snapshot,
	snapshotLevelsMap map[string][]sbe.SnapshotLevelsList,
	key string,
) Event {
	event := models.OrderBookRawNotification{
		Timestamp:      int64(snapshot.TimestampMs),
		InstrumentName: instrumentName,
		ChangeID:       int64(snapshot.ChangeId),
	}

	if levelsList, ok := snapshotLevelsMap[key]; ok {
		snapshot.LevelsList = append(levelsList, snapshot.LevelsList...)
		delete(snapshotLevelsMap, key)
	}

	for _, level := range snapshot.LevelsList {
		item := models.OrderBookNotificationItem{
			Action: "new",
			Price:  level.Price,
			Amount: level.Amount,
		}

		if level.Side == sbe.BookSide.Ask {
			event.Asks = append(event.Asks, item)
		} else if level.Side == sbe.BookSide.Bid {
			event.Bids = append(event.Bids, item)
		}
	}
	return Event{
		Type: EventTypeSnapshot,
		Data: event,
	}
}

func (c *Client) emitEvents(events []Event) {
	for _, event := range events {
		switch event.Type {
		case EventTypeInstrument:
			// update instrumentsMap
			ins := event.Data.(models.Instrument)
			c.setInstrument(ins.InstrumentID, ins)

			// emit event
			c.Emit(newInstrumentNotificationChannel(ins.Kind, ins.BaseCurrency), &ins)
			c.Emit(newInstrumentNotificationChannel(KindAny, ins.BaseCurrency), &ins)

		case EventTypeOrderBook:
			books := event.Data.(models.OrderBookRawNotification)
			c.Emit(newOrderBookNotificationChannel(books.InstrumentName), &books)

		case EventTypeTrades:
			trades := event.Data.(models.TradesNotification)
			if len(trades) > 0 {
				tradeIns := trades[0].InstrumentName
				tradeKind := trades[0].InstrumentKind
				currency := getCurrencyFromInstrument(tradeIns)
				c.Emit(newTradesNotificationChannel(tradeKind, currency), &trades)
			}

		case EventTypeTicker:
			ticker := event.Data.(models.TickerNotification)
			c.Emit(newTickerNotificationChannel(ticker.InstrumentName), &ticker)

		case EventTypeSnapshot:
			snapshot := event.Data.(models.OrderBookRawNotification)
			c.Emit(newSnapshotNotificationChannel(snapshot.InstrumentName), &snapshot)
		}
	}
}

// Start starts listening events on interface `ifname` and `addrs`.
func (c *Client) Start(ctx context.Context) error {
	err := c.buildInstrumentsMapping()
	if err != nil {
		c.log.Errorw("failed to build instruments mapping", "err", err)
		return err
	}

	return c.ListenToEvents(ctx)
}

// restartConnection should re-build instrumentMap.
func (c *Client) restartConnections(ctx context.Context) error {
	err := c.Stop()
	if err != nil {
		return err
	}

	err = c.Start(ctx)
	if err != nil {
		return err
	}

	c.Emit(RestartEventChannel, true)
	return nil
}

// Stop stops listening for events.
func (c *Client) Stop() error {
	for _, conn := range c.connMap {
		conn.Close()
	}

	c.connMap = nil

	return nil
}

func (c *Client) getInstrument(id uint32) models.Instrument {
	return c.instrumentsMap[id]
}

func (c *Client) setInstrument(id uint32, ins models.Instrument) {
	c.instrumentsMap[id] = ins
}

func readPackageHeader(reader io.Reader) (uint16, uint16, uint32, error) {
	b8 := make([]byte, 8)
	if _, err := io.ReadFull(reader, b8); err != nil {
		return 0, 0, 0, err
	}

	n := uint16(b8[0]) | uint16(b8[1])<<8
	channelID := uint16(b8[2]) | uint16(b8[3])<<8
	seq := uint32(b8[4]) | uint32(b8[5])<<8 | uint32(b8[6])<<16 | uint32(b8[7])<<24
	return n, channelID, seq, nil
}

func (c *Client) handlePackageHeader(reader io.Reader, chanelIDSeq map[uint16]uint32) error {
	_, channelID, seq, err := readPackageHeader(reader)
	if err != nil {
		c.log.Errorw("failed to decode events", "err", err)
		return err
	}

	lastSeq, ok := chanelIDSeq[channelID]

	if ok {
		log := c.log.With("channelID", channelID, "current_seq", seq, "last_seq", lastSeq)
		if seq == 0 && math.MaxUint32-lastSeq >= 2 {
			return ErrConnectionReset
		}
		if seq == lastSeq {
			log.Debugw("package duplicated")
			return ErrDuplicatedPackage
		}
		if seq != lastSeq+1 {
			log.Warnw("package out of order")
		}
	}

	chanelIDSeq[channelID] = seq
	return nil
}

// Handle decodes an UDP packages into events.
// If it receives an InstrumentChange event, it has to update the instrumentsMap accordingly.
// This function needs to handle for missing package, a jump in sequence number, e.g: prevSeq=5, curSeq=7
// And needs to handle for connection reset, the sequence number is zero.
// Note that the sequence number go from max(uint32) to zero is a normal event, not a connection reset.
func (c *Client) Handle(
	marshaller *sbe.SbeGoMarshaller,
	reader io.Reader,
	chanelIDSeq map[uint16]uint32,
	bookChangesMap map[string][]sbe.BookChangesList,
	snapshotLevelsMap map[string][]sbe.SnapshotLevelsList,
) error {
	err := c.handlePackageHeader(reader, chanelIDSeq)
	if err != nil {
		if errors.Is(err, ErrDuplicatedPackage) {
			return nil
		}
		c.log.Errorw("failed to handle package header", "err", err)
		return err
	}

	events, err := c.decodeEvents(marshaller, reader, bookChangesMap, snapshotLevelsMap)
	if err != nil {
		c.log.Errorw("failed to decode events", "err", err)
		return err
	}

	c.emitEvents(events)
	return nil
}

type Bytes []byte

type Pool struct {
	mu            *sync.RWMutex
	queue         []Bytes
	maxPacketSize int
}

func NewPool(maxPacketSize int) *Pool {
	q := make([]Bytes, 0)

	return &Pool{
		mu:            &sync.RWMutex{},
		queue:         q,
		maxPacketSize: maxPacketSize,
	}
}

func (p *Pool) Get() Bytes {
	p.mu.Lock()
	defer p.mu.Unlock()

	if n := len(p.queue); n > 0 {
		item := p.queue[n-1]
		p.queue[n-1] = nil
		p.queue = p.queue[:n-1]
		return item
	}
	return make(Bytes, p.maxPacketSize)
}

func (p *Pool) Put(bs Bytes) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.queue = append(p.queue, bs[:cap(bs)])
}

func (c *Client) setupConnection(port int, ips []string) ([]net.IP, error) {
	lc := net.ListenConfig{
		Control: Control,
	}
	conn, err := lc.ListenPacket(context.Background(), "udp4", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		c.log.Errorw("Failed to initiate packet connection", "err", err, "port", port)
		return nil, err
	}

	c.connMap[port] = ipv4.NewPacketConn(conn)

	ipGroups := make([]net.IP, len(ips))

	err = c.connMap[port].SetControlMessage(ipv4.FlagDst, true)
	if err != nil {
		c.log.Errorw("Failed to set control message", "err", err)
		return nil, err
	}

	for i, ip := range ips {
		group := net.ParseIP(ip)
		if group == nil {
			return nil, ErrInvalidIpv4Address
		}
		err := c.connMap[port].JoinGroup(c.inf, &net.UDPAddr{IP: group})
		if err != nil {
			c.log.Errorw("failed to join group", "group", group, "err", err, "ip", ip)
			return nil, err
		}
		ipGroups[i] = group
	}

	return ipGroups, nil
}

func splitAddr(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, errors.New("invalid address")
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}

	return parts[0], port, nil
}

func (c *Client) setupConnections() (map[int][]net.IP, error) {
	c.connMap = make(map[int]*ipv4.PacketConn)

	portIPsMap := make(map[int][]string)
	for _, addr := range c.addrs {
		ip, port, err := splitAddr(addr)
		if err != nil {
			c.log.Errorw("Fail to split addr", "addr", addr, "error", err)
			return nil, err
		}

		ips := portIPsMap[port]
		ips = append(ips, ip)
		portIPsMap[port] = ips
	}

	portNetIPsMap := make(map[int][]net.IP)
	for port, ips := range portIPsMap {
		newIPs, err := c.setupConnection(port, ips)
		if err != nil {
			c.log.Errorw("Fail to setup connection", "port", port, "ips", ips)
			return nil, err
		}

		portNetIPsMap[port] = newIPs
	}

	return portNetIPsMap, nil
}

// ListenToEvents listens to a list of udp addresses on given network interface.
//
//nolint:cyclop,gocognit
func (c *Client) ListenToEvents(ctx context.Context) error {
	portIPsMap, err := c.setupConnections()
	if err != nil {
		c.log.Errorw("failed to setup ipv4 packet connection", "err", err)
		return err
	}

	dataCh := make(chan []byte, defaultDataChSize)
	pool := NewPool(maxPacketSize)

	// handle data from dataCh
	go func() {
		m := sbe.NewSbeGoMarshaller()
		channelIDSeq := make(map[uint16]uint32)
		bookChangesMap := make(map[string][]sbe.BookChangesList)
		snapshotLevelsMap := make(map[string][]sbe.SnapshotLevelsList)
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-dataCh:
				if !ok {
					return
				}
				err := c.handleUDPPackage(ctx, m, channelIDSeq, data, bookChangesMap, snapshotLevelsMap)
				if err != nil {
					c.log.Errorw("Fail to handle UDP package", "error", err)
				}
				pool.Put(data)
			}
		}
	}()

	// listen to event using ipv4 package
	for port, conn := range c.connMap {
		go func(conn *ipv4.PacketConn, ips []net.IP) {
			for {
				data := pool.Get()

				res, err := readUDPMulticastPackage(conn, ips, data)
				if res == nil {
					pool.Put(data)
				}

				if err != nil {
					if isNetConnClosedErr(err) {
						c.log.Infow("Connection closed", "error", err)
						closeChannel(dataCh)
						break
					}
					c.log.Errorw("Fail to read UDP multicast package", "error", err)
				} else if res != nil {
					dataCh <- res
				}
			}
		}(conn, portIPsMap[port])
	}

	return nil
}

func (c *Client) handleUDPPackage(
	ctx context.Context,
	m *sbe.SbeGoMarshaller,
	channelIDSeq map[uint16]uint32,
	data []byte,
	bookChangesMap map[string][]sbe.BookChangesList,
	snapshotLevelsMap map[string][]sbe.SnapshotLevelsList,
) error {
	buf := bytes.NewBuffer(data)
	err := c.Handle(m, buf, channelIDSeq, bookChangesMap, snapshotLevelsMap)
	if err != nil {
		if errors.Is(err, ErrConnectionReset) {
			if err = c.restartConnections(ctx); err != nil {
				c.log.Error("failed to restart connections", "err", err)
				return err
			}
		} else {
			c.log.Errorw(
				"Fail to handle UDP package",
				"data", base64.StdEncoding.EncodeToString(data),
				"error", err,
			)
			return err
		}
	}

	return nil
}

func readUDPMulticastPackage(conn *ipv4.PacketConn, ipGroups []net.IP, data []byte) ([]byte, error) {
	n, cm, _, err := conn.ReadFrom(data)
	if err != nil {
		return nil, err
	}

	if cm.Dst.IsMulticast() {
		if checkValidDstAddress(cm.Dst, ipGroups) {
			return data[:n], nil
		}
	}

	return nil, nil
}

func checkValidDstAddress(dest net.IP, groups []net.IP) bool {
	for _, ipGroup := range groups {
		if dest.Equal(ipGroup) {
			return true
		}
	}
	return false
}

func closeChannel(ch chan []byte) {
	defer func() {
		_ = recover()
	}()

	close(ch)
}
