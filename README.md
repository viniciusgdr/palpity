# Palpity — Go SDK para Rodovia (5 minutos)

Pacote Go que conecta ao mercado **Rodovia (5 minutos): quantos carros?** do [Palpity](https://app.palpity.io/) via WebSocket em tempo real.

Recebe eventos de contagem de carros, mudanças de odds, apostas, encerramento de rodada e transição automática entre rodadas.

## Instalação

```bash
go get github.com/viniciusgdr/palpity
```

## Uso Rápido

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os/signal"
    "syscall"

    "github.com/viniciusgdr/palpity/pkg/palpity"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    client := palpity.New(
        palpity.WithEvents(palpity.EventCarCount | palpity.EventSettlement),
    )

    client.OnCarCount = func(e palpity.CarCountEvent) {
        fmt.Printf("Total de carros: %d\n", e.CurrentTotal)
    }

    client.OnSettlement = func(e palpity.SettlementEvent) {
        fmt.Printf("Rodada encerrada! Vencedor: %s\n", e.WinnerLabel)
    }

    if err := client.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Consultar o Mercado Atual

Use `GetStatus()` quando você só quiser abrir a conexão, pegar o status atual do mercado e devolver esse snapshot para o seu código.

```go
status, err := palpity.GetStatus(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Faltam %.0fs para fechar o mercado\n", status.TimeUntilClose.Seconds())
fmt.Printf("Rodovia: %s\n", status.RoadInfo())
for _, side := range status.Selections {
    fmt.Printf("%s -> odd: %.2f (%s%%)\n", side.Label, side.Odd, side.Percent)
}
```

Use `WatchStatus()` se quiser receber novos snapshots automaticamente sempre que uma nova rodada começar ou quando as odds forem atualizadas.

```go
err := palpity.WatchStatus(ctx, func(status palpity.MarketStatus) {
    fmt.Printf("Mercado %d | faltam %.0fs\n", status.MarketID, status.TimeUntilClose.Seconds())
    for _, side := range status.Selections {
        fmt.Printf("%s -> odd: %.2f (%s%%)\n", side.Label, side.Odd, side.Percent)
    }
})
if err != nil {
    log.Fatal(err)
}
```

Se você estiver usando um `Client` manualmente, também pode consultar `CurrentStatus()` ou `CurrentMarket()`. Todos esses acessos usam o último estado sincronizado pelo WebSocket, e o tempo restante é recalculado na hora da consulta.

## Eventos Disponíveis

Cada evento pode ser ativado individualmente usando `WithEvents()`. Use `|` (OR) para combinar:

```go
palpity.WithEvents(palpity.EventCarCount | palpity.EventOddsUpdate)
```

Use `palpity.EventAll` para receber todos os eventos.

### `EventCarCount` — Contagem de Carros

Disparado toda vez que a IA detecta um novo carro na câmera da rodovia.

| Campo          | Tipo     | Descrição                           |
| -------------- | -------- | ----------------------------------- |
| `ID`           | `int`    | Identificador único da detecção     |
| `Value`        | `string` | Incremento (sempre `"1"`)           |
| `CurrentTotal` | `int`    | Total acumulado de carros na rodada |
| `Timestamp`    | `int64`  | Unix timestamp da detecção          |

```go
client.OnCarCount = func(e palpity.CarCountEvent) {
    fmt.Printf("Carro detectado! Total: %d\n", e.CurrentTotal)
}
```

### `EventOddsUpdate` — Atualização de Odds

Disparado quando as odds/probabilidades mudam (geralmente após uma aposta ou quando a contagem se aproxima da meta).

| Campo        | Tipo                | Descrição                         |
| ------------ | ------------------- | --------------------------------- |
| `MarketID`   | `int`               | ID do mercado                     |
| `Slug`       | `string`            | Slug do mercado                   |
| `UpdatedAt`  | `string`            | Timestamp ISO 8601 da atualização |
| `Selections` | `[]SelectionUpdate` | Lista com as seleções atualizadas |

Cada `SelectionUpdate`:

| Campo           | Tipo     | Descrição                                      |
| --------------- | -------- | ---------------------------------------------- |
| `SelectionID`   | `int`    | ID da seleção                                  |
| `SelectionCode` | `string` | Código único (ex: `"19050_MAIS_DE_86"`)        |
| `Label`         | `string` | Label legível (ex: `"Mais de 86"`, `"Até 86"`) |
| `Percent`       | `string` | Probabilidade em percentual (ex: `"58"`)       |
| `Odd`           | `string` | Odd decimal (ex: `"1.72"`)                     |

```go
client.OnOddsUpdate = func(e palpity.OddsUpdateEvent) {
    for _, s := range e.Selections {
        fmt.Printf("%s -> odd: %s (%s%%)\n", s.Label, s.Odd, s.Percent)
    }
}
```

### `EventSettlement` — Rodada Encerrada

Disparado quando a rodada de 5 minutos termina e o vencedor é definido.

| Campo         | Tipo     | Descrição                                        |
| ------------- | -------- | ------------------------------------------------ |
| `MarketID`    | `int`    | ID do mercado encerrado                          |
| `Slug`        | `string` | Slug do mercado                                  |
| `UpdatedAt`   | `string` | Timestamp ISO 8601                               |
| `WinnerLabel` | `string` | Label do resultado vencedor (ex: `"Mais de 86"`) |

```go
client.OnSettlement = func(e palpity.SettlementEvent) {
    fmt.Printf("Vencedor: %s\n", e.WinnerLabel)
}
```

### `EventTrade` — Nova Aposta

Disparado quando alguém faz uma aposta no mercado.

| Campo       | Tipo        | Descrição          |
| ----------- | ----------- | ------------------ |
| `MarketID`  | `int`       | ID do mercado      |
| `Slug`      | `string`    | Slug do mercado    |
| `UpdatedAt` | `string`    | Timestamp ISO 8601 |
| `Data`      | `TradeData` | Dados da aposta    |

`TradeData`:

| Campo     | Tipo      | Descrição                               |
| --------- | --------- | --------------------------------------- |
| `Amount`  | `float64` | Valor apostado em reais                 |
| `LabelID` | `int`     | ID da seleção escolhida                 |
| `Color`   | `string`  | Cor da seleção (RGB, ex: `"226,56,56"`) |

```go
client.OnTrade = func(e palpity.TradeEvent) {
    fmt.Printf("Aposta de R$%.2f na seleção %d\n", e.Data.Amount, e.Data.LabelID)
}
```

### `EventChartUpdate` — Atualização do Gráfico

Disparado quando os dados do gráfico de probabilidades são atualizados.

| Campo       | Tipo               | Descrição          |
| ----------- | ------------------ | ------------------ |
| `MarketID`  | `int`              | ID do mercado      |
| `Slug`      | `string`           | Slug do mercado    |
| `UpdatedAt` | `string`           | Timestamp ISO 8601 |
| `Data`      | `[]ChartSelection` | Dados por seleção  |

`ChartSelection`:

| Campo   | Tipo           | Descrição         |
| ------- | -------------- | ----------------- |
| `ID`    | `int`          | ID da seleção     |
| `Label` | `string`       | Label da seleção  |
| `Data`  | `[]ChartPoint` | Pontos do gráfico |

`ChartPoint`:

| Campo  | Tipo      | Descrição                               |
| ------ | --------- | --------------------------------------- |
| `Date` | `int64`   | Unix timestamp do ponto                 |
| `Prob` | `string`  | Probabilidade no momento (ex: `"57.9"`) |
| `Odd`  | `float64` | Odd no momento                          |

```go
client.OnChartUpdate = func(e palpity.ChartUpdateEvent) {
    for _, s := range e.Data {
        if len(s.Data) > 0 {
            last := s.Data[len(s.Data)-1]
            fmt.Printf("%s: prob=%s%% odd=%.2f\n", s.Label, last.Prob, last.Odd)
        }
    }
}
```

### `EventNewRound` — Nova Rodada

Disparado quando uma nova rodada começa (inclui a primeira ao conectar). Recebe o struct `Market` completo.

| Campo                     | Tipo          | Descrição                                     |
| ------------------------- | ------------- | --------------------------------------------- |
| `ID`                      | `int`         | ID do mercado                                 |
| `Slug`                    | `string`      | Slug do mercado                               |
| `Title`                   | `string`      | Título (ex: `"Rodovia (5 min): quantos?"`)    |
| `Description`             | `string`      | Texto completo de regras e descrição do mercado |
| `ClosesAt`                | `time.Time`   | Horário de encerramento                       |
| `RemainingSeconds`        | `float64`     | Segundos restantes na rodada                  |
| `RemainingBettingSeconds` | `float64`     | Segundos restantes para apostar               |
| `Metadata.ValueNeeded`    | `int`         | Meta de carros (ex: `86`)                     |
| `Metadata.Tag`            | `string`      | Identificador da câmera (ex: `"SP123-KM046"`) |
| `Selections`              | `[]Selection` | Opções disponíveis com odds iniciais          |

```go
client.OnNewRound = func(m palpity.Market) {
    fmt.Printf("Rodovia: %s\n", m.RoadInfo())
    fmt.Printf("Nova rodada! Meta: %d carros\n", m.Metadata.ValueNeeded)
    for _, s := range m.Selections {
        fmt.Printf("  %s -> odd: %.2f (%s%%)\n", s.Label, s.Odd, s.Percent)
    }
}
```

## Opções

| Opção                      | Descrição                                         |
| -------------------------- | ------------------------------------------------- |
| `WithEvents(EventType)`    | Define quais eventos receber (padrão: `EventAll`) |
| `WithLogger(*slog.Logger)` | Logger customizado (padrão: `slog.Default()`)     |

## Métodos Úteis

| Método            | Descrição                                                         |
| ----------------- | ----------------------------------------------------------------- |
| `CurrentStatus()` | Retorna um snapshot enxuto com odds atuais, tempo restante e meta |
| `CurrentMarket()` | Retorna o snapshot completo do mercado atual                      |

`Market` e `MarketStatus` também expõem:

- `RoadInfo()`: retorna a primeira linha útil da descrição. Ex.: `Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP).`
- `RoadName()`: retorna só o nome da rodovia. Ex.: `Floriano Rodrigues Pinheiro`

## Funções Independentes

| Função          | Descrição                                                                    |
| --------------- | ---------------------------------------------------------------------------- |
| `GetStatus()`   | Conecta, captura um único snapshot atual do mercado e devolve esse resultado |
| `WatchStatus()` | Conecta e entrega novos snapshots ao longo da sessão                         |

## Comportamento Interno

- Conecta automaticamente ao WebSocket Pusher do Palpity
- Ao final de cada rodada (`settlement`), busca automaticamente o próximo mercado
- Reconexão automática com backoff exponencial (1s a 30s) em caso de desconexão
- A conexão WebSocket é mantida entre rodadas (apenas os canais mudam)
- `Start()` bloqueia até o `context` ser cancelado

## Executar o Exemplo

```bash
go run ./cmd/example/
```
