.PHONY: up down logs test vet integration crypto-test proto e2e

up:
	docker compose up -d --build --wait

down:
	docker compose down

logs:
	docker compose logs -f api web

test:
	go test ./...

vet:
	go vet ./...

integration:
	@test -n "$$TEST_DATABASE_URL" || { echo "Set TEST_DATABASE_URL; see README.md"; exit 1; }
	go test -race ./...

crypto-test:
	mkdir -p build/crypto
	$(CXX) -std=c++20 -Wall -Wextra -Werror -fsanitize=address,undefined services/Encryption_Service/kuznechik/tests/known_answer_test.cpp services/Encryption_Service/kuznechik/src/kuznechik/kuznechik.cpp -o build/crypto/known-answer
	./build/crypto/known-answer

proto:
	sh scripts/generate-proto.sh

e2e:
	docker run --rm --init --shm-size=256m --network diploma_default -e WEB_URL=http://web -v "$(CURDIR)/scripts:/work:ro" -w /tmp mcr.microsoft.com/playwright:v1.64.0-noble sh -ec 'npm install --prefix /tmp/browser playwright@1.64.0 --no-audit --no-fund >/dev/null; NODE_PATH=/tmp/browser/node_modules node /work/browser-smoke.cjs'
