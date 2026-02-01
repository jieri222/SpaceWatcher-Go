# SpaceWatcher-Go

Twitter/X Space 下載工具，使用 Go 編寫。

## 功能

- 🎙️ 下載已結束的 Twitter Space 錄音
- ⏳ 直播中的 Space 會自動等待結束後下載
- 📝 自訂檔名格式
- 🎵 自動嵌入 m4a metadata (標題/創建者)
- ⚡ 並行下載，速度快
- 🔧 自動提取 API 參數，無需手動設定

## 安裝

從 [Releases](https://github.com/jieri222/SpaceWatcher-Go/releases) 下載對應平台的執行檔。

或使用 Go 安裝：

```bash
go install github.com/jieri222/SpaceWatcher-Go/cmd@latest
```

## 使用方式

```bash
# 基本用法
spacewatcher https://x.com/i/spaces/xxxxxxxxxxxxx

# 自訂輸出檔名
spacewatcher -o my_space.m4a https://x.com/i/spaces/xxxxx

# 自訂檔名格式
spacewatcher -o "{date}_{creator_name}_{title}.m4a" https://x.com/i/spaces/xxxxx

# 詳細輸出
spacewatcher -v https://x.com/i/spaces/xxxxx
```

> ⚠️ **注意**: 所有選項必須放在 URL 之前

## 參數說明

| 參數 | 短 | 說明 | 預設值 |
|------|-----|------|--------|
| `--output` | `-o` | 輸出檔案路徑/格式 | `{date}_{title}.m4a` |
| `--concurrency` | `-c` | 下載併發數 | `5` |
| `--retry` | `-r` | 重試次數 | `3` |
| `--interval` | `-i` | 等待時的檢查間隔（秒） | `30` |
| `--verbose` | `-v` | 顯示詳細 log | `false` |

## 檔名格式變數

| 變數 | 說明 | 範例 |
|------|------|------|
| `{date}` | 開始日期 | `20260117` |
| `{time}` | 開始時間 | `210000` |
| `{datetime}` | 日期時間 | `20260117_210000` |
| `{title}` | Space 標題 | `我的直播` |
| `{creator_name}` | 創建者名稱 | `用戶名` |
| `{creator_screen_name}` | 創建者 @ 帳號 | `@username` |
| `{spaceID}` | Space ID | `1mnxeNapBrvKX` |

## 系統需求

- [FFmpeg](https://ffmpeg.org/) 必須安裝並加入 PATH

## 授權

MIT License
