PROTOC=protoc
PROTO_DIR?=../api/grpc/proto
OUT_DIR?=../api/grpc/pb

# Файлы .proto
PROTO_FILES=$(wildcard $(PROTO_DIR)/*.proto)

$(OUT_DIR)/%.pb.go: $(PROTO_DIR)/%.proto
	@mkdir -p $(OUT_DIR)
	$(PROTOC)   --proto_path=$(PROTO_DIR) --go_out=$(OUT_DIR) --go_opt=paths=source_relative --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative $<


genProto: $(patsubst $(PROTO_DIR)/%.proto, $(OUT_DIR)/%.pb.go, $(PROTO_FILES))

clean:
	rm -rf $(OUT_DIR)/*
