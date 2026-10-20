package model

import "testing"

func TestAssignResultURLNeverStoresProxyPath(t *testing.T) {
	task := &Task{TaskID: "task_proxy"}
	task.AssignResultURL("https://api.example.test/v1/videos/task_proxy/content")
	if task.PrivateData.ResultURL != "" {
		t.Fatalf("proxy path stored as result: %q", task.PrivateData.ResultURL)
	}
	task.AssignResultURL("/v1/tasks/task_proxy/artifacts/file/content")
	if task.PrivateData.ResultURL != "" {
		t.Fatalf("artifact proxy path stored as result: %q", task.PrivateData.ResultURL)
	}

	task.AssignResultURL(" https://cdn.example/video.mp4 ")
	if task.PrivateData.ResultURL != "https://cdn.example/video.mp4" {
		t.Fatalf("direct url = %q", task.PrivateData.ResultURL)
	}
	task.AssignResultURL("https://api.example.test/v1/videos/task_proxy/content")
	if task.PrivateData.ResultURL != "https://cdn.example/video.mp4" {
		t.Fatalf("proxy write overwrote a real url: %q", task.PrivateData.ResultURL)
	}

	task.PrivateData.ResultURL = "https://api.example.test/v1/videos/task_proxy/content"
	task.AssignResultURL("")
	if task.PrivateData.ResultURL != "" {
		t.Fatalf("stored proxy path was not cleared: %q", task.PrivateData.ResultURL)
	}

	task.AssignResultURL(" data:video/mp4;base64,AAAA ")
	if task.PrivateData.ResultURL != "data:video/mp4;base64,AAAA" {
		t.Fatalf("data url = %q", task.PrivateData.ResultURL)
	}
}

func TestIsProxyResultURLMatchesFallbackPathsOnly(t *testing.T) {
	if !IsProxyResultURL("https://api.example.test/v1/videos/task_1/content?download=1", "task_1") {
		t.Fatal("video content path should be a proxy result")
	}
	if !IsProxyResultURL("/v1/tasks/task_1/artifacts/out/content", "task_1") {
		t.Fatal("artifact content path should be a proxy result")
	}
	if IsProxyResultURL("https://cdn.example/video.mp4", "task_1") {
		t.Fatal("direct media url is not a proxy result")
	}
	if IsProxyResultURL("https://api.example.test/v1/videos/other/content", "task_1") {
		t.Fatal("another task's content path is not this task's proxy result")
	}
}
