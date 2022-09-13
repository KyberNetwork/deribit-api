package fix

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type MockInitiator struct {
}

func CreateMockInitiator() *MockInitiator {
	return &MockInitiator{}
}

func (i *MockInitiator) Start() error {
	return nil
}

func (i *MockInitiator) Stop() {}

type FixTestSuite struct {
	suite.Suite
	c *Client
}

func TestFixTestSuite(t *testing.T) {
	suite.Run(t, new(FixTestSuite))
}

func (ts *FixTestSuite) SetupSuite() {
	ts.c = &Client{}
}

func (ts *FixTestSuite) TestHandleSubscriptions() {}

func (ts *FixTestSuite) TestSend() {}

func (ts *FixTestSuite) TestCall() {}

func (ts *FixTestSuite) TestMarketDataRequest() {}

func (ts *FixTestSuite) TestSubscribeOrderBooks() {}

func (ts *FixTestSuite) TestUnsubscribeOrderBooks() {}

func (ts *FixTestSuite) TestSubscribeTrades() {}

func (ts *FixTestSuite) TestUnsubscribeTrades() {}

func (ts *FixTestSuite) TestSubscribe() {}

func (ts *FixTestSuite) TestUnsubscribe() {}

func (ts *FixTestSuite) TestCreateOrder() {}
