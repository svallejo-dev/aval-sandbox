# aval-sandbox

Sandbox público para validar [aval](https://github.com/svallejo-dev/aval) de punta a punta con PRs reales. **No es un servicio real:** no lo despliegues ni dependas de él.

## Qué es

`orders` es un microservicio REST pequeño en Go que gestiona pedidos y sus pagos en memoria. Tiene lo justo para que aval tenga algo realista que gobernar:

- **Specs de OpenSpec** en `openspec/specs/`. Cada requirement empieza por un ID de obligación del contexto `ORD` (ADR-0002 de aval): `ORD-F01 Order total equals the sum of its lines`.
- **Tests vinculados.** Cada obligación F, N o I tiene al menos un test cuyo subtest empieza por su ID: `t.Run("ORD-F01 …", …)`. Hay tests de tabla, tests HTTP con `httptest` y dos tests de máquina de estados con [rapid](https://pkg.go.dev/pgregory.net/rapid) para los invariantes.
- **Contrato OpenAPI** en `api/openapi.yaml`. Cada operación lista sus obligaciones en `x-requirements`.
- **Manifiesto de aval** en `aval.yaml`: contexto `ORD`, modo `enforce` y las familias de rutas `dx`, `feat` y `seam`.

## Obligaciones

| ID | Obligación |
|---|---|
| ORD-F01 | Order total equals the sum of its lines |
| ORD-F02 | Order changes status along allowed edges |
| ORD-F03 | Order is readable by its ID (caracterización) |
| ORD-F04 | Creating an order returns its total |
| ORD-N01 | Order total is never negative |
| ORD-N02 | Closed orders never change status |
| ORD-N03 | Lines are added only while pending |
| ORD-I01 | Order invariants hold for any sequence |
| ORD-F10 | Refund is idempotent per key |
| ORD-F11 | Payment captures a positive amount |
| ORD-F12 | Refund requires an idempotency key |
| ORD-N10 | Never refund more than captured |
| ORD-N11 | Refund key never changes its amount |
| ORD-I10 | Refunded total never exceeds captured |

La fuente de verdad son las specs; esta tabla es solo un índice.

## Estructura

Package Oriented Design. El dominio solo usa la biblioteca estándar, y `depguard` lo impone en el lint.

```
cmd/orders/                   main: señales y arranque, nada más
internal/app/                 composition root: repositorios, handlers y rutas
internal/order/               dominio: agregado Order (líneas, total, estados)
internal/payment/             dominio: agregado Payment (captura, reembolsos idempotentes)
internal/orderhttp/           adaptador HTTP de pedidos
internal/paymenthttp/         adaptador HTTP de pagos
internal/ordermem/            repositorio de pedidos en memoria
internal/paymentmem/          repositorio de pagos en memoria
internal/platform/httpserver/ servidor HTTP con apagado ordenado y convenciones JSON
```

## Cómo se ejecuta

Requiere las versiones de `.tool-versions` (Go 1.27.1 y golangci-lint 2.13.2, con asdf).

```bash
make build     # bin/orders
make test      # go test -race ./...
make lint      # golangci-lint
make verify    # las tres cosas: lo que corre el CI

ORDERS_ADDR=:8080 ./bin/orders
```

```bash
curl -s -XPOST localhost:8080/orders \
  -d '{"lines":[{"sku":"BOOK","quantity":2,"unitPrice":1500}]}'
curl -s -XPOST localhost:8080/payments -d '{"orderId":"<id>","amount":3000}'
curl -s -XPOST localhost:8080/payments/<id>/refunds \
  -H 'Idempotency-Key: k1' -d '{"amount":500}'
```

El dinero va en unidades menores (céntimos), como entero.

## Con aval

```bash
aval trace --plain              # matriz obligación → test
aval trace --run-tests --plain  # además ejecuta go test y muestra el resultado de cada test vinculado
```

Las specs se validan con `openspec validate --all --strict`, usando la versión exacta de `aval.yaml` (`@fission-ai/openspec@1.13.1`).

### Familias de rutas

`aval.yaml` reparte las rutas entre `dx`, `feat` y `seam`. El código que se ejecuta en producción nunca es `dx`:

- **feat:** `internal/**`, `api/**` y `openspec/**`. Incluye `internal/platform/**`, porque su comportamiento lo ve el cliente (límite del cuerpo, decodificación JSON, cuerpo de los errores).
- **seam:** `cmd/**` e `internal/app/app.go`, que solo conectan las piezas.
- **dx:** tooling, CI e instrucciones de agentes (`AGENTS.md`, `CLAUDE.md`, `.claude/**`, `.agents/**`).

Un commit que toca `dx` y `feat` a la vez es `mixed` y bloquea.

### Política base y PRs de validación

El gate lee la política **del SHA base** (ADR-0005 de aval). Un PR que edita el `aval.yaml` raíz, `.aval/baseline.json`, `.github/**`, `CODEOWNERS` o `.golangci.yml` produce `tamper` (`policy_edited`) y bloquea.

Las etiquetas `aval:override` y `aval:human-approved` son solo informativas: el gate lee los reviews del PR, no las etiquetas.

Resultados esperados de los PRs de validación:

- **PR 1:**
  - El test nuevo tiene que **compilar contra la base**. La falla-antes se ejecuta superponiendo el test sobre la base: si allí no compila, la evidencia es `weak` y el gate da `warn` (`weak_evidence`) en lugar de evidencia fuerte.
  - Tampoco debe tocar `internal/app/app.go`: es `seam` y añadiría el aviso `seam_touched`.
- **PR 5** (cambia `mode` en `aval.yaml`): se espera que **bloquee** con `tamper` (`policy_edited`). El cambio de modo no tiene efecto, porque el modo sale de la política de la base.

## Licencia

[MIT](LICENSE)
