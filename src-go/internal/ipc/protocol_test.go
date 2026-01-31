package ipc

import (
	"encoding/json"
	"testing"
)

func TestEncodeRequest(t *testing.T) {
	req := &Request{
		Cmd:   CmdPlay,
		Token: "test-token",
	}

	data, err := EncodeRequest(req)
	if err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}

	// Verify it's valid JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if decoded["cmd"] != "play" {
		t.Errorf("Expected cmd 'play', got '%v'", decoded["cmd"])
	}

	if decoded["token"] != "test-token" {
		t.Errorf("Expected token 'test-token', got '%v'", decoded["token"])
	}
}

func TestDecodeRequest(t *testing.T) {
	data := []byte(`{"cmd":"pause","token":"my-token"}`)

	req, err := DecodeRequest(data)
	if err != nil {
		t.Fatalf("DecodeRequest failed: %v", err)
	}

	if req.Cmd != CmdPause {
		t.Errorf("Expected cmd 'pause', got '%s'", req.Cmd)
	}

	if req.Token != "my-token" {
		t.Errorf("Expected token 'my-token', got '%s'", req.Token)
	}
}

func TestDecodeRequestWithData(t *testing.T) {
	data := []byte(`{"cmd":"play","token":"tok","data":{"path":"/music/song.mp3"}}`)

	req, err := DecodeRequest(data)
	if err != nil {
		t.Fatalf("DecodeRequest failed: %v", err)
	}

	if req.Cmd != CmdPlay {
		t.Errorf("Expected cmd 'play', got '%s'", req.Cmd)
	}

	var playReq PlayRequest
	if err := json.Unmarshal(req.Data, &playReq); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if playReq.Path != "/music/song.mp3" {
		t.Errorf("Expected path '/music/song.mp3', got '%s'", playReq.Path)
	}
}

func TestDecodeRequestInvalid(t *testing.T) {
	data := []byte(`not valid json`)

	_, err := DecodeRequest(data)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestEncodeResponse(t *testing.T) {
	resp := &Response{
		Success: true,
	}

	data, err := EncodeResponse(resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if decoded["success"] != true {
		t.Errorf("Expected success true, got %v", decoded["success"])
	}
}

func TestDecodeResponse(t *testing.T) {
	data := []byte(`{"success":true,"data":{"state":"playing"}}`)

	resp, err := DecodeResponse(data)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success to be true")
	}

	if resp.Data == nil {
		t.Error("Expected data to be non-nil")
	}
}

func TestDecodeResponseError(t *testing.T) {
	data := []byte(`{"success":false,"error":"unauthorized"}`)

	resp, err := DecodeResponse(data)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	if resp.Success {
		t.Error("Expected success to be false")
	}

	if resp.Error != "unauthorized" {
		t.Errorf("Expected error 'unauthorized', got '%s'", resp.Error)
	}
}

func TestNewSuccessResponse(t *testing.T) {
	statusData := StatusResponse{
		State:    "playing",
		Position: 1000,
		Duration: 180000,
	}

	resp, err := NewSuccessResponse(statusData)
	if err != nil {
		t.Fatalf("NewSuccessResponse failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success to be true")
	}

	if resp.Data == nil {
		t.Error("Expected data to be non-nil")
	}

	// Verify data can be decoded back
	var decoded StatusResponse
	if err := json.Unmarshal(resp.Data, &decoded); err != nil {
		t.Fatalf("Failed to decode data: %v", err)
	}

	if decoded.State != "playing" {
		t.Errorf("Expected state 'playing', got '%s'", decoded.State)
	}
}

func TestNewSuccessResponseNilData(t *testing.T) {
	resp, err := NewSuccessResponse(nil)
	if err != nil {
		t.Fatalf("NewSuccessResponse failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success to be true")
	}

	if resp.Data != nil {
		t.Error("Expected data to be nil")
	}
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("something went wrong")

	if resp.Success {
		t.Error("Expected success to be false")
	}

	if resp.Error != "something went wrong" {
		t.Errorf("Expected error 'something went wrong', got '%s'", resp.Error)
	}
}

func TestCommandTypes(t *testing.T) {
	commands := []CommandType{
		CmdPair,
		CmdPlay,
		CmdPause,
		CmdResume,
		CmdStop,
		CmdNext,
		CmdPrev,
		CmdQueue,
		CmdSeek,
		CmdVolume,
		CmdStatus,
	}

	for _, cmd := range commands {
		// Verify each command serializes correctly
		req := &Request{Cmd: cmd}
		data, err := EncodeRequest(req)
		if err != nil {
			t.Errorf("Failed to encode %s: %v", cmd, err)
		}

		decoded, err := DecodeRequest(data)
		if err != nil {
			t.Errorf("Failed to decode %s: %v", cmd, err)
		}

		if decoded.Cmd != cmd {
			t.Errorf("Expected %s, got %s", cmd, decoded.Cmd)
		}
	}
}

func TestPlayRequest(t *testing.T) {
	playReq := PlayRequest{
		Path: "/music/song.mp3",
		Metadata: &TrackMetadata{
			Title:    "Test Song",
			Artist:   "Test Artist",
			Album:    "Test Album",
			Duration: 180000,
		},
	}

	data, err := json.Marshal(playReq)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded PlayRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Path != "/music/song.mp3" {
		t.Errorf("Expected path '/music/song.mp3', got '%s'", decoded.Path)
	}

	if decoded.Metadata == nil {
		t.Fatal("Expected metadata to be non-nil")
	}

	if decoded.Metadata.Title != "Test Song" {
		t.Errorf("Expected title 'Test Song', got '%s'", decoded.Metadata.Title)
	}
}

func TestQueueRequest(t *testing.T) {
	queueReq := QueueRequest{
		Items: []QueueItem{
			{Path: "/song1.mp3"},
			{Path: "/song2.mp3"},
		},
		Append: true,
	}

	data, err := json.Marshal(queueReq)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded QueueRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(decoded.Items))
	}

	if decoded.Items[0].Path != "/song1.mp3" {
		t.Errorf("Expected first path to be /song1.mp3, got %s", decoded.Items[0].Path)
	}

	if !decoded.Append {
		t.Error("Expected Append to be true")
	}
}

func TestSeekRequest(t *testing.T) {
	seekReq := SeekRequest{Position: 30000}

	data, err := json.Marshal(seekReq)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded SeekRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Position != 30000 {
		t.Errorf("Expected position 30000, got %d", decoded.Position)
	}
}

func TestVolumeRequest(t *testing.T) {
	volReq := VolumeRequest{Level: 0.75}

	data, err := json.Marshal(volReq)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded VolumeRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Level != 0.75 {
		t.Errorf("Expected level 0.75, got %f", decoded.Level)
	}
}

func TestStatusResponse(t *testing.T) {
	status := StatusResponse{
		State:      "playing",
		Path:       "/music/current.mp3",
		Position:   15000,
		Duration:   180000,
		Volume:     0.8,
		QueueIndex: 2,
		QueueSize:  10,
		Metadata: &TrackMetadata{
			Title:  "Current Song",
			Artist: "Current Artist",
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded StatusResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.State != "playing" {
		t.Errorf("Expected state 'playing', got '%s'", decoded.State)
	}

	if decoded.QueueIndex != 2 {
		t.Errorf("Expected queue index 2, got %d", decoded.QueueIndex)
	}

	if decoded.QueueSize != 10 {
		t.Errorf("Expected queue size 10, got %d", decoded.QueueSize)
	}
}

func TestPairRequest(t *testing.T) {
	pairReq := PairRequest{ClientName: "VS Code Extension"}

	data, err := json.Marshal(pairReq)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded PairRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ClientName != "VS Code Extension" {
		t.Errorf("Expected client name 'VS Code Extension', got '%s'", decoded.ClientName)
	}
}

func TestPairResponse(t *testing.T) {
	pairResp := PairResponse{
		Token:            "generated-token-123",
		ClientID:         "client-456",
		RequiresApproval: true,
	}

	data, err := json.Marshal(pairResp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded PairResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Token != "generated-token-123" {
		t.Errorf("Expected token 'generated-token-123', got '%s'", decoded.Token)
	}

	if decoded.ClientID != "client-456" {
		t.Errorf("Expected client ID 'client-456', got '%s'", decoded.ClientID)
	}

	if !decoded.RequiresApproval {
		t.Error("Expected RequiresApproval to be true")
	}
}
