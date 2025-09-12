package mocks

// Lightweight local stubs to satisfy imports in integration tests.

type MobiusMockServer struct{}

func NewMobiusMockServer(args ...string) *MobiusMockServer { return &MobiusMockServer{} }
func (m *MobiusMockServer) GetBaseURL() string             { return "http://localhost:18080" }
func (m *MobiusMockServer) EnableWebhooks(url string)      {}
func (m *MobiusMockServer) Stop()                          {}
func (m *MobiusMockServer) Close()                         {}

type MobiusClientMock struct{}

func NewMobiusClientMock() *MobiusClientMock               { return &MobiusClientMock{} }
func (c *MobiusClientMock) SetResponse(name string, v any) {}

type CCBillMockServer struct{}

func NewCCBillMockServer() *CCBillMockServer { return &CCBillMockServer{} }
func (c *CCBillMockServer) Close()           {}
