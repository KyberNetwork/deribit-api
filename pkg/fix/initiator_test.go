package fix

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/tag"
)

type MockInitiator struct {
	app             quickfix.Application
	settings        *quickfix.Settings
	sessionSettings map[quickfix.SessionID]*quickfix.SessionSettings
	storeFactory    quickfix.MessageStoreFactory
	logFactory      quickfix.LogFactory
	results         []interface{}
	stopChan        chan interface{}
}

func createMockInitiator(
	app quickfix.Application,
	storeFactory quickfix.MessageStoreFactory,
	appSettings *quickfix.Settings,
	logFactory quickfix.LogFactory,
) (Initiator, error) {
	i := &MockInitiator{
		app:             app,
		storeFactory:    storeFactory,
		settings:        appSettings,
		sessionSettings: appSettings.SessionSettings(),
		logFactory:      logFactory,
		results: []interface{}{
			&models.AuthResponse{},
			"success",
		},
	}
	for sessionID := range i.sessionSettings {
		app.OnCreate(sessionID)
	}

	return i, nil
}

func (i *MockInitiator) Start() error {
	i.stopChan = make(chan interface{})

	for sessionID, s := range i.sessionSettings {
		if !s.HasSetting("SocketConnectHost") {
			return errors.New("Conditionally Required Setting: SocketConnectHost")
		}

		if !s.HasSetting("SocketConnectPort") {
			return errors.New("Conditionally Required Setting: SocketConnectPort")
		}
		i.app.OnLogon(sessionID)
	}

	return nil
}

func (i *MockInitiator) Stop() {
	select {
	case <-i.stopChan:
		for sessionID := range i.sessionSettings {
			i.app.OnLogout(sessionID)
		}
		return
	default:
	}
	close(i.stopChan)
}

// Send sends message to counterparty (Deribit Server)
func (i *MockInitiator) send(msg *quickfix.Message) error {
	msgType, err := msg.Header.GetBytes(tag.MsgType)
	if err != nil {
		fmt.Println("xx", err)
		return err
	}

	reqIDTag, err2 := getReqIDTagFromMsgType(enum.MsgType(msgType))
	if err2 != nil {
		// for creating order, requestID dont get from msgType
		fmt.Println("yy", err2)
		reqIDTag = tag.ClOrdID
		// return err2
	}

	mutex.Lock()
	requestID, err = msg.Body.GetString(reqIDTag)
	mutex.Unlock()
	if err != nil {
		fmt.Println("zz", err)
		return err
	}

	if isAdminMessageType(msgType) {
		for sessionID := range i.sessionSettings {
			i.app.ToAdmin(msg, sessionID)
		}
	} else {
		for sessionID := range i.sessionSettings {
			err := i.app.ToApp(msg, sessionID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Receive receives message from counterparty (Deribit Server)
func (i *MockInitiator) receive(msg *quickfix.Message) error {
	msgType, err := msg.Header.GetBytes(tag.MsgType)
	if err != nil {
		return err
	}

	if isAdminMessageType(msgType) {
		for sessionID := range i.sessionSettings {
			err := i.app.FromAdmin(msg, sessionID)
			if err != nil {
				return err
			}
		}
	} else {
		for sessionID := range i.sessionSettings {
			err := i.app.FromApp(msg, sessionID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func mockDeribitResponse(msg *quickfix.Message) (err error) {
	initiator := mockInitiator.(*MockInitiator)
	return initiator.receive(msg)
}

func mockSender(m quickfix.Messagable) (err error) {
	initiator := mockInitiator.(*MockInitiator)
	return initiator.send(m.ToMessage())
}

// nolint:gochecknoglobals
var (
	msgTypeHeartbeat     = []byte("0")
	msgTypeLogon         = []byte("A")
	msgTypeTestRequest   = []byte("1")
	msgTypeResendRequest = []byte("2")
	msgTypeReject        = []byte("3")
	msgTypeSequenceReset = []byte("4")
	msgTypeLogout        = []byte("5")
)

// isAdminMessageType returns true if the message type is a session level message.
func isAdminMessageType(m []byte) bool {
	switch {
	case bytes.Equal(msgTypeHeartbeat, m),
		bytes.Equal(msgTypeLogon, m),
		bytes.Equal(msgTypeTestRequest, m),
		bytes.Equal(msgTypeResendRequest, m),
		bytes.Equal(msgTypeReject, m),
		bytes.Equal(msgTypeSequenceReset, m),
		bytes.Equal(msgTypeLogout, m):
		return true
	}

	return false
}
