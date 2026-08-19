//go:build darwin

package swiftbridge

import _ "embed"

// dylibEmbed 内嵌打包后的 Swift 桥接库（与 build.sh 产出、本目录下的 libkai_bridge.dylib 同源）。
// 非开发态（打包产物）运行时从本字节落地临时文件再 Dlopen，不依赖外部文件存在；
// 开发态优先使用本地 pkg/swiftbridge/libkai_bridge.dylib（改 Swift 重编即生效，无需重编 Go 二进制）。
//
//go:embed libkai_bridge.dylib
var dylibEmbed []byte
