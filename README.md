# Primind Notification Throttling Management

通知のスロットリング

## エンドポイント

| メソッド | エンドポイント | 概要 |
|---------|------|------|
| POST | /api/v1/throttle | 通知スロットル処理 |
| POST | /api/v1/throttle/batch | バッチスロットル処理 |
| POST | /api/v1/remind/cancel | リマインダーキャンセル |
| GET | /health | ヘルスチェック |
| GET | /metrics | Prometheusメトリクス |

## Proto定義

- `proto/throttle/v1/throttle.proto`
- `proto/notify/v1/notify.proto`
- `proto/common/v1/common.proto`
  - Enum: `TaskType`

## 依存

- Redis v8
- Remind Time Management
- InfluxDB / BigQuery
