.PHONY: build build-backend build-frontend build-datamanagementd test test-backend test-frontend test-frontend-critical test-datamanagementd secret-scan \
        dev-deps dev-deps-down dev-deps-logs dev-backend dev-frontend

FRONTEND_CRITICAL_VITEST := \
	src/views/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/views/auth/__tests__/WechatCallbackView.spec.ts \
	src/views/user/__tests__/PaymentView.spec.ts \
	src/views/user/__tests__/PaymentResultView.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@pnpm --dir frontend run build

# 编译 datamanagementd（宿主机数据管理进程）
build-datamanagementd:
	@cd datamanagement && go build -o datamanagementd ./cmd/datamanagementd

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@pnpm --dir frontend exec vitest run $(FRONTEND_CRITICAL_VITEST)

test-datamanagementd:
	@cd datamanagement && go test ./...

secret-scan:
	@python3 tools/secret_scan.py

# =============================================================================
# 本地联调（三件套）
# 一次性配置：在 deploy/.env 写入 POSTGRES_PASSWORD=sub2api（或留空走默认值）
# 三个终端分别跑：
#   make dev-deps       # 后端依赖（postgres + redis）跑在 docker
#   make dev-backend    # 后端 go run（热改代码重启即可）
#   make dev-frontend   # 前端 vite dev（HMR）
# =============================================================================
DEV_COMPOSE := docker compose -f deploy/docker-compose.deps.yml
DEV_ENV_FILE := deploy/.env

dev-deps:
	@$(DEV_COMPOSE) up -d
	@echo "✔ postgres @ 127.0.0.1:5432  redis @ 127.0.0.1:6379"

dev-deps-down:
	@$(DEV_COMPOSE) down

dev-deps-logs:
	@$(DEV_COMPOSE) logs -f

dev-backend:
	@set -a; [ -f $(DEV_ENV_FILE) ] && . ./$(DEV_ENV_FILE); set +a; \
	 PORT=$${SERVER_PORT:-8090}; \
	 PIDS=$$(lsof -ti tcp:$$PORT 2>/dev/null || true); \
	 if [ -n "$$PIDS" ]; then echo "⚠ 端口 $$PORT 被占用，清理旧进程：$$PIDS"; kill -9 $$PIDS 2>/dev/null || true; sleep 1; fi; \
	 cd backend && \
	   DATABASE_HOST=127.0.0.1 \
	   DATABASE_PORT=$${DATABASE_PORT:-5432} \
	   DATABASE_USER=$${POSTGRES_USER:-sub2api} \
	   DATABASE_PASSWORD=$${POSTGRES_PASSWORD:-sub2api} \
	   DATABASE_DBNAME=$${POSTGRES_DB:-sub2api} \
	   DATABASE_SSLMODE=disable \
	   REDIS_HOST=127.0.0.1 \
	   REDIS_PORT=$${REDIS_PORT:-6379} \
	   REDIS_PASSWORD=$${REDIS_PASSWORD:-} \
	   SERVER_HOST=0.0.0.0 SERVER_PORT=$${SERVER_PORT:-8090} SERVER_MODE=debug \
	   AUTO_SETUP=true \
	   go run ./cmd/server/

dev-frontend:
	@set -a; [ -f $(DEV_ENV_FILE) ] && . ./$(DEV_ENV_FILE); set +a; \
	 PORT=$${VITE_DEV_PORT:-3000}; \
	 PIDS=$$(lsof -ti tcp:$$PORT 2>/dev/null || true); \
	 if [ -n "$$PIDS" ]; then echo "⚠ 端口 $$PORT 被占用，清理旧进程：$$PIDS"; kill -9 $$PIDS 2>/dev/null || true; sleep 1; fi; \
	 pnpm --dir frontend dev
