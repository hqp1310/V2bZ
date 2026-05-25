# ZicNode
A ZicBoard backend based on modified xray-core.
Một backend của ZicBoard dựa trên mã nguồn xray-core đã được chỉnh sửa.

**Lưu ý: Dự án này cần được sử dụng kết hợp với [ZicBoard](https://github.com/ZicBoard/ZicBoard)**

## Cài đặt phần mềm

### Cài đặt nhanh bằng 1 click

```bash
wget -N https://raw.githubusercontent.com/kutycma/zicnode/main/script/install.sh && bash install.sh
```

## Biên dịch (Build)
```bash
GOEXPERIMENT=jsonv2 go build -v -o build_assets/zicnode -trimpath -ldflags "-X 'github.com/ZicBoard/ZicNode/cmd.version=$version' -s -w -buildid="
```

## Lịch sử Stars

[![Stargazers over time](https://starchart.cc/kutycma/zicnode.svg?variant=adaptive)](https://starchart.cc/kutycma/zicnode)
