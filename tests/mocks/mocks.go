package mocks

// Lightweight local stubs to satisfy imports in integration tests.

type NMIMockServer struct{}

func NewNMIMockServer(args ...string) *NMIMockServer { return &NMIMockServer{} }
func (m *NMIMockServer) GetBaseURL() string          { return "http://localhost:18080" }
func (m *NMIMockServer) EnableWebhooks(url string)   {}
func (m *NMIMockServer) Stop()                       {}
func (m *NMIMockServer) Close()                      {}

type NMIClientMock struct{}

func NewNMIClientMock() *NMIClientMock                  { return &NMIClientMock{} }
func (c *NMIClientMock) SetResponse(name string, v any) {}

type CCBillMockServer struct{}

func NewCCBillMockServer() *CCBillMockServer { return &CCBillMockServer{} }
func (c *CCBillMockServer) Close()           {}
