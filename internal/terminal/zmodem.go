package terminal

import (
	"errors"
	"os"
	"path/filepath"
)

// ZMODEM 协议常量
const (
	zmodemZDLE = 0x18 // ZDLE 转义字符 (CAN)
	zmodemZPAD = '*'  // ZMODEM 起始填充字符
)

// ZmodemReceiver 负责检测和处理 ZMODEM 文件传输。
//
// 这是一个简化实现，仅支持接收 sz 发送的文件。
// 工作流程：
//  1. SSH 读循环将数据传入 Detect() 检测 ZMODEM 起始序列
//  2. 检测到后进入 ZMODEM 模式，后续数据通过 Process() 处理
//  3. Process() 解析文件名、大小，并写入本地文件
//  4. 传输完成后退出 ZMODEM 模式
type ZmodemReceiver struct {
	active   bool   // 是否处于 ZMODEM 接收模式
	fileName string // 当前接收的文件名
	fileSize int64  // 当前接收的文件大小
	received int64  // 已接收的字节数
	file     *os.File
	saveDir  string // 文件保存目录

	// 内部缓冲区，用于跨数据块匹配 ZMODEM 序列
	buf      []byte
	inZFile  bool // 是否正在解析 ZFILE 头
	inZData  bool // 是否正在接收文件数据
	zfileBuf []byte
	zdataBuf []byte
}

// NewZmodemReceiver 创建一个 ZMODEM 接收器。
// saveDir 指定文件保存目录，为空则使用当前目录。
func NewZmodemReceiver() *ZmodemReceiver {
	return &ZmodemReceiver{
		saveDir: ".",
	}
}

// SetSaveDir 设置文件保存目录。
func (z *ZmodemReceiver) SetSaveDir(dir string) {
	z.saveDir = dir
}

// Active 返回是否处于 ZMODEM 接收模式。
func (z *ZmodemReceiver) Active() bool {
	return z.active
}

// FileName 返回当前接收的文件名。
func (z *ZmodemReceiver) FileName() string {
	return z.fileName
}

// FileSize 返回当前接收的文件大小。
func (z *ZmodemReceiver) FileSize() int64 {
	return z.fileSize
}

// Received 返回已接收的字节数。
func (z *ZmodemReceiver) Received() int64 {
	return z.received
}

// Detect 检测数据流中是否包含 ZMODEM 起始序列（ZRINIT）。
// 返回 true 表示检测到 ZMODEM 传输开始，调用方应将后续数据
// 交给 Process() 处理而非显示在终端上。
//
// 检测的起始序列为：**\x18B00000000000000（ZRINIT）
// 实际 sz 发送的是 ZRQINIT，格式为：**\x18B0100000023be50
func (z *ZmodemReceiver) Detect(data []byte) bool {
	if z.active {
		return true
	}

	// 将新数据追加到缓冲区
	z.buf = append(z.buf, data...)
	// 防止缓冲区无限增长：只保留最后 64 字节
	if len(z.buf) > 64 {
		z.buf = z.buf[len(z.buf)-64:]
	}

	// 查找 ZRQINIT/ZRINIT 起始标志：**\x18B
	for i := 0; i+4 <= len(z.buf); i++ {
		if z.buf[i] == zmodemZPAD && z.buf[i+1] == zmodemZPAD && z.buf[i+2] == zmodemZDLE && z.buf[i+3] == 'B' {
			// 找到 ZMODEM 起始序列
			z.active = true
			z.buf = z.buf[:0] // 清空缓冲区，开始接收
			return true
		}
	}

	return false
}

// Process 处理 ZMODEM 数据。
// 返回值：
//   - []byte：非 ZMODEM 数据（传输结束后需要显示在终端上的数据）
//   - error：处理错误
//
// 该方法解析 ZFILE 帧（获取文件名和大小），
// 然后写入 ZDATA 帧中的文件数据。
func (z *ZmodemReceiver) Process(data []byte) ([]byte, error) {
	if !z.active {
		return data, nil
	}

	var leftover []byte
	z.buf = append(z.buf, data...)

	for len(z.buf) > 0 {
		if !z.inZFile && !z.inZData {
			// 寻找下一个帧
			idx, frameType, frameData := z.parseFrame()
			if idx < 0 {
				break // 数据不完整，等待更多数据
			}

			switch frameType {
			case "ZFILE":
				z.inZFile = true
				z.zfileBuf = append(z.zfileBuf[:0], frameData...)
				if err := z.handleZFile(); err != nil {
					return nil, err
				}
				z.inZFile = false
				z.inZData = true
			case "ZDATA":
				z.inZData = true
				z.zdataBuf = append(z.zdataBuf[:0], frameData...)
				if err := z.handleZData(); err != nil {
					return nil, err
				}
			case "ZEOF", "ZFINISH":
				// 传输结束
				z.finishTransfer()
				leftover = append(leftover, z.buf[idx:]...)
				z.buf = z.buf[:0]
				return leftover, nil
			}
			z.buf = z.buf[idx:]
		} else {
			break
		}
	}

	return nil, nil
}

// parseFrame 尝试从缓冲区解析一个 ZMODEM 帧。
// 返回：已消费的字节数、帧类型、帧数据。
// 如果数据不完整返回 -1。
//
// 这是一个简化的帧解析器，不完整实现 ZMODEM 协议。
func (z *ZmodemReceiver) parseFrame() (int, string, []byte) {
	// 查找帧起始：**\x18B（二进制帧）
	for i := 0; i+4 <= len(z.buf); i++ {
		// 二进制帧：**\x18B 后跟 4 字节类型 + 4 字节标志
		if z.buf[i] == zmodemZPAD && z.buf[i+1] == zmodemZPAD && z.buf[i+2] == zmodemZDLE && z.buf[i+3] == 'B' {
			// 至少需要 4(头) + 8(十六进制数据) + 2(CRC) = 14 字节
			if i+4+8+2 > len(z.buf) {
				return -1, "", nil
			}
			// 解析帧类型（前4个十六进制字符）和标志（后4个）
			typeHex := string(z.buf[i+4 : i+8])
			// 简化处理：根据类型码判断帧类型
			frameType := z.frameTypeFromHex(typeHex)
			// 查找帧结束（回车换行或下一个帧起始）
			endIdx := z.findFrameEnd(i + 4)
			if endIdx < 0 {
				return -1, "", nil
			}
			return endIdx, frameType, z.buf[i+4 : endIdx]
		}
	}
	return -1, "", nil
}

// frameTypeFromHex 根据十六进制类型码返回帧类型名称。
func (z *ZmodemReceiver) frameTypeFromHex(hex string) string {
	switch hex {
	case "0000":
		return "ZRQINIT"
	case "0001":
		return "ZRINIT"
	case "0002":
		return "ZSINIT"
	case "0003":
		return "ZACK"
	case "0004":
		return "ZFILE"
	case "0005":
		return "ZSKIP"
	case "0006":
		return "ZNAK"
	case "0007":
		return "ZABORT"
	case "0008":
		return "ZFINISH"
	case "0009":
		return "ZRPOS"
	case "000a":
		return "ZDATA"
	case "000b":
		return "ZEOF"
	case "000c":
		return "ZFERR"
	case "000d":
		return "ZCRC"
	case "000e":
		return "ZCHALLENGE"
	case "000f":
		return "ZCOMPL"
	case "0010":
		return "ZCAN"
	default:
		return "UNKNOWN"
	}
}

// findFrameEnd 在缓冲区中查找帧结束位置。
// ZMODEM 帧以 CRC + 帧结束符（回车/换行）结尾。
func (z *ZmodemReceiver) findFrameEnd(start int) int {
	for i := start; i < len(z.buf); i++ {
		// 检测下一个帧起始或 CAN 序列
		if i+4 <= len(z.buf) && z.buf[i] == zmodemZPAD && z.buf[i+1] == zmodemZPAD && z.buf[i+2] == zmodemZDLE {
			return i
		}
		// 检测 ZMODEM 结束序列（连续 5 个 CAN）
		if i+5 <= len(z.buf) && z.buf[i] == zmodemZDLE && z.buf[i+1] == zmodemZDLE &&
			z.buf[i+2] == zmodemZDLE && z.buf[i+3] == zmodemZDLE && z.buf[i+4] == zmodemZDLE {
			return i + 5
		}
	}
	// 如果没找到明确结束，返回整个缓冲区（简化处理）
	return len(z.buf)
}

// handleZFile 解析 ZFILE 帧获取文件名和大小。
// ZFILE 帧数据格式：filename filesize modtime filemode \0
func (z *ZmodemReceiver) handleZFile() error {
	// ZFILE 帧的数据部分包含文件信息
	// 格式：filename filesize [modtime filemode]\0
	// 这里简化解析：跳过帧头，提取文件名和大小
	data := z.zfileBuf
	// 跳过帧类型和标志（前8个十六进制字符）
	if len(data) < 8 {
		return errors.New("zmodem: ZFILE 帧数据过短")
	}

	// 在实际 ZMODEM 实现中，ZFILE 的数据在后续的数据子包中。
	// 这里简化处理：直接从帧数据中解析。
	// 实际 sz 发送的格式更复杂，这里仅做基本解析。
	info := string(data[8:])
	parts := splitZmodemFields(info)
	if len(parts) >= 2 {
		z.fileName = parts[0]
		z.fileSize = parseInt64(parts[1])
	}

	// 创建文件
	path := filepath.Join(z.saveDir, z.fileName)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	z.file = f
	z.received = 0
	return nil
}

// handleZData 处理文件数据帧，将数据写入文件。
func (z *ZmodemReceiver) handleZData() error {
	if z.file == nil {
		return errors.New("zmodem: ZDATA 帧在 ZFILE 之前")
	}

	// ZDATA 帧数据格式：offset(8 hex) + 数据
	data := z.zdataBuf
	if len(data) < 8 {
		return nil
	}

	// 跳过偏移量（8个十六进制字符），写入实际数据
	fileData := z.unescape(data[8:])
	if len(fileData) > 0 {
		n, err := z.file.Write(fileData)
		if err != nil {
			return err
		}
		z.received += int64(n)
	}
	return nil
}

// unescape 处理 ZMODEM 转义序列。
// ZMODEM 使用 ZDLE (0x18) 作为转义字符：
//   - ZDLE 0x40 -> 0x00
//   - ZDLE 0x41 -> 0x01
//   - ... (控制字符转义)
//   - ZDLE 0x4a -> 0x0a
//   - ZDLE 0x4d -> 0x0d
//   - ZDLE 0x5a -> 0x1a
//   - ZDLE 0x60 -> 0x40 (ZDLE 本身)
func (z *ZmodemReceiver) unescape(data []byte) []byte {
	result := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == zmodemZDLE && i+1 < len(data) {
			// ZDLE 转义
			i++
			c := data[i]
			if c&0x60 == 0x40 {
				result = append(result, c&0x1f)
			} else if c&0x60 == 0x20 {
				result = append(result, c|0x40)
			} else {
				result = append(result, c)
			}
		} else {
			result = append(result, data[i])
		}
	}
	return result
}

// finishTransfer 完成文件传输，关闭文件并重置状态。
func (z *ZmodemReceiver) finishTransfer() {
	if z.file != nil {
		z.file.Close()
		z.file = nil
	}
	z.active = false
	z.inZFile = false
	z.inZData = false
	z.zfileBuf = z.zfileBuf[:0]
	z.zdataBuf = z.zdataBuf[:0]
	z.buf = z.buf[:0]
	z.fileName = ""
	z.fileSize = 0
	z.received = 0
}

// Cancel 取消正在进行的 ZMODEM 传输。
func (z *ZmodemReceiver) Cancel() {
	if z.file != nil {
		z.file.Close()
		z.file = nil
		// 删除未完成的文件
		if z.fileName != "" {
			os.Remove(filepath.Join(z.saveDir, z.fileName))
		}
	}
	z.finishTransfer()
}

// splitZmodemFields 按空格和空字符分割 ZMODEM 字段。
func splitZmodemFields(s string) []string {
	var fields []string
	current := ""
	for _, c := range s {
		if c == ' ' || c == 0 || c == '\t' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else if c < 0x20 {
			// 控制字符，跳过
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}

// parseInt64 将字符串解析为 int64，失败返回 0。
func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
