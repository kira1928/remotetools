# 工具可执行性与 isExecutable 字段

本页介绍 remotetools 如何处理工具的“可执行性”，以及 `isExecutable` 配置项的用法。

## isExecutable 字段

- 类型：布尔，可选；默认值：`true`
- 作用：指示该条目是否为“可直接执行”的程序（例如 ELF/EXE、带可执行入口的脚本）。
- 适用：
  - 设为 `false` 用于诸如 AnyCPU 的 `.dll`、纯资源包、或仅供其他工具调用的库文件等无法直接执行的项目。

配置示例：

```
{
  "dotnet": {
    "8.0.5": {
      "downloadUrl": "...",
      "pathToEntry": "...",
      "printInfoCmd": ["--info"],
      "isExecutable": true
    }
  },
  "some-dll-tool": {
    "1.0.0": {
      "downloadUrl": "...",
      "pathToEntry": "lib/Some.dll",
      "isExecutable": false
    }
  }
}
```

## 安装后的执行性检测与临时目录

当 `isExecutable = true` 时，安装完成后 remotetools 会执行“执行性检测”：

1. 在工具的存储目录中进行一次“可执行”检查（Linux 上会尝试创建并执行一个简单脚本，Windows 上默认视为可执行）。
2. 若存储目录不可执行，且配置了 `tmpExecRootFolder`，则会将工具复制到该临时目录并再次检查。
3. 如果在临时目录仍无法执行，则安装流程会被标记为失败。

注意：
- `tmpExecRootFolder` 可通过 API `SetTmpRootFolderForExecPermission(path)` 设置。
- 复制到临时目录失败或检测失败时，remotetools 会自动清理临时目录，避免留下脏数据。
- WebUI 会在工具卡片“版本”行显示“临时目录运行”的徽标，以便识别该工具当前从临时目录执行。

## WebUI 与 isExecutable

- 列表项会包含 `isExecutable` 字段，前端在其为 `false` 时不展示“查看信息”按钮，而是显示“非可执行程序”文字。
- 标题行旁会显示当前平台信息（如 `linux/amd64`）。

## 常见问题

- 问：为何 Windows 不进行执行权限检测？
  - 答：Windows 的执行模型不同于 POSIX，NTFS 不具备可执行位，使用统一脚本检测容易误判。因此在 Windows 上默认视为“可执行”，实际的运行时问题会在工具调用时暴露。

- 问：复制到临时目录会影响更新卸载吗？
  - 答：不会。卸载工具会同时尝试清理临时执行目录中的对应副本。
