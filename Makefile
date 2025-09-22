# Makefile para Momentum E-commerce (Microsserviços em Go)

# Caminhos
SHARED_PATH=shared

# Variáveis

.PHONY: clean proto up down

clean:
	@echo "==> Limpando binários..."
	rm -f $(IDENTITY_PATH)/$(BINARY_NAME)

proto:
	@echo "==> Gerando código Go a partir dos protos..."
	protoc -I=$(SHARED_PATH) --go_out=$(SHARED_PATH) --go-grpc_out=$(SHARED_PATH) $(SHARED_PATH)/protobuf/identity.proto --experimental_allow_proto3_optional

up:
	@echo "==> Subindo stack com Docker Compose..."
	docker compose up --build

down:
	@echo "==> Derrubando stack Docker Compose..."
	docker compose down
