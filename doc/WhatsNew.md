2026-06-15(v0.8.0)
- 新增 Web 代理选择器：在页面中查看可用代理列表，一键选择/取消
- 新增本地混合代理端口：SOCKS5 + HTTP CONNECT，流量通过选中节点转发
- 新增选择后自动测试连通性，页面实时显示结果
- 新增 retry_with_proxy：用健康代理重试直连失败的 server_url
- 新增配置项 proxy_port、retry_with_proxy、retry_max_proxies
- API 端点：GET /api/proxies, GET /api/selected, POST /api/select, POST /api/unselect

2021-06-19(v0.3.1)
- 忽略source的TLS证书校验 #24
- http请求添加timeout #24
- fix: 一个url请求失败后后续全都失败 #24

2021-06-03
- 添加健康检测并发数设置(v0.3.0)
- 更新依赖，修复 send on closed channel

2021-04-20
- 添加自定义timeout
- 更严格的有效性检查标准


2020-11-14
- 可以分离web界面显示的端口与实际serve端口
- 增加测速和筛选
- 改变代码结构
- 重写测速，增加自定义timeout