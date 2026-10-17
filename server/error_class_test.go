package server

import (
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/pipeline"
)

func TestDeriveErrorClass(t *testing.T) {
	cases := []struct {
		name string
		out  pipeline.Outcome
		want string
	}{
		{
			name: "network from receipt err",
			out:  pipeline.Outcome{Receipts: []contract.Receipt{{OK: false, Tool: "exec", Err: "dial tcp: connection refused"}}, BizReason: "动作未成功：dial tcp: connection refused"},
			want: "network_error",
		},
		{
			name: "model from receipt err",
			out:  pipeline.Outcome{Receipts: []contract.Receipt{{OK: false, Tool: "llm", Err: "empty content; finish=length"}}, BizReason: "动作未成功：empty content"},
			want: "model_error",
		},
		{
			name: "execution default",
			out:  pipeline.Outcome{Receipts: []contract.Receipt{{OK: false, Tool: "exec", Err: "exit status 1"}}, BizReason: "动作未成功：exit status 1"},
			want: "execution_error",
		},
		{
			name: "verify fail is execution",
			out:  pipeline.Outcome{BizReason: "验证失败：内容不匹配预期"},
			want: "execution_error",
		},
		{
			name: "nil outcome",
			want: "execution_error",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveErrorClass(&c.out); got != c.want {
				t.Fatalf("deriveErrorClass = %q, want %q", got, c.want)
			}
		})
	}
}
