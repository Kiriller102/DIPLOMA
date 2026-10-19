PROTOC=protoc
PROTO_DIR?=../api/grpc/proto
OUT_DIR?=../api/grpc/pb
THIRD_PARTY_DIR=../api/grpc/third_party

# Файлы .proto
PROTO_FILES=$(wildcard $(PROTO_DIR)/*.proto)
PROTO_NAMES=$(notdir $(PROTO_FILES:.proto=))

$(OUT_DIR)/%/%.pb.go: $(PROTO_DIR)/%.proto
	@mkdir -p $(OUT_DIR)/$*
	$(PROTOC) -I $(PROTO_DIR) -I $(THIRD_PARTY_DIR)/googleapis --proto_path=$(PROTO_DIR) --go_out=$(OUT_DIR)/$* --go_opt=paths=source_relative --go-grpc_out=$(OUT_DIR)/$* --go-grpc_opt=paths=source_relative $<


genProto: $(patsubst $(PROTO_DIR)/%.proto, $(OUT_DIR)/%/%.pb.go, $(PROTO_FILES))

clean:
	rm -rf $(OUT_DIR)/*
