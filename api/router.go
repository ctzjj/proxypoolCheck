package api

import (
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ssrlive/proxypool/pkg/healthcheck"
	"github.com/ssrlive/proxypool/pkg/provider"
	"github.com/ssrlive/proxypoolCheck/config"
	"github.com/ssrlive/proxypoolCheck/internal/app"
	appcache "github.com/ssrlive/proxypoolCheck/internal/cache"
	"github.com/gin-contrib/cache"
	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-gonic/gin"
)

const version = "v0.7.3"

var router *gin.Engine

func setupRouter() {
	gin.SetMode(gin.ReleaseMode)
	router = gin.New() // 没有任何中间件的路由
	store := persistence.NewInMemoryStore(time.Minute)
	router.Use(gin.Recovery(), cache.SiteCache(store, time.Minute))

	_ = RestoreAssets("", "assets/html")
	_ = RestoreAssets("", "assets/css")

	injectDrawer("assets/html/index.html")

	temp, err := loadHTMLTemplate()
	if err != nil {
		panic(err)
	}
	router.SetHTMLTemplate(temp)
	router.StaticFile("/css/index.css", "assets/css/index.css")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "assets/html/index.html", gin.H{
			"domain":               config.Config.Domain,
			"request":              config.Config.Request,
			"port":                 config.Config.Port,
			"all_proxies_count":    appcache.AllProxiesCount,
			"ss_proxies_count":     appcache.SSProxiesCount,
			"ssr_proxies_count":    appcache.SSRProxiesCount,
			"vmess_proxies_count":  appcache.VmessProxiesCount,
			"trojan_proxies_count": appcache.TrojanProxiesCount,
			"useful_proxies_count": appcache.UsableProxiesCount,
			"last_crawl_time":      appcache.LastCrawlTime,
			"version":              version,
		})
	})
	router.GET("/clash", func(c *gin.Context) {
		c.HTML(http.StatusOK, "assets/html/clash.html", gin.H{
			"domain":  config.Config.Domain,
			"port":    config.Config.Port,
			"request": config.Config.Request,
		})
	})

	router.GET("/surge", func(c *gin.Context) {
		c.HTML(http.StatusOK, "assets/html/surge.html", gin.H{
			"domain":  config.Config.Domain,
			"request": config.Config.Request,
			"port":    config.Config.Port,
		})
	})

	router.GET("/clash/config", func(c *gin.Context) {
		c.HTML(http.StatusOK, "assets/html/clash-config.yaml", gin.H{
			"domain":  config.Config.Domain,
			"request": config.Config.Request,
			"port":    config.Config.Port,
		})
	})
	router.GET("/clash/localconfig", func(c *gin.Context) {
		c.HTML(http.StatusOK, "assets/html/clash-config-local.yaml", gin.H{
			"port": config.Config.Port,
		})
	})
	router.GET("/clash/proxies", func(c *gin.Context) {
		proxyTypes := c.DefaultQuery("type", "")
		proxyCountry := c.DefaultQuery("c", "")
		proxyNotCountry := c.DefaultQuery("nc", "")
		proxySpeed := c.DefaultQuery("speed", "")
		proxyFilter := c.DefaultQuery("filter", "")
		text := ""
		if proxyTypes == "" && proxyCountry == "" && proxyNotCountry == "" && proxySpeed == "" && proxyFilter == "" {
			text = appcache.GetString("clashproxies") // A string. To show speed in this if condition, this must be updated after speedtest
			if text == "" {
				proxies := appcache.GetProxies("proxies")
				clash := provider.Clash{
					Base: provider.Base{
						Proxies: &proxies,
					},
				}
				text = clash.Provide() // 根据Query筛选节点
				appcache.SetString("clashproxies", text)
			}
		} else if proxyTypes == "all" {
			proxies := appcache.GetProxies("allproxies")
			clash := provider.Clash{
				provider.Base{
					Proxies:    &proxies,
					Types:      proxyTypes,
					Country:    proxyCountry,
					NotCountry: proxyNotCountry,
					Speed:      proxySpeed,
					Filter:     proxyFilter,
				},
			}
			text = clash.Provide() // 根据Query筛选节点
		} else {
			proxies := appcache.GetProxies("proxies")
			clash := provider.Clash{
				provider.Base{
					Proxies:    &proxies,
					Types:      proxyTypes,
					Country:    proxyCountry,
					NotCountry: proxyNotCountry,
					Speed:      proxySpeed,
					Filter:     proxyFilter,
				},
			}
			text = clash.Provide() // 根据Query筛选节点
		}
		c.String(200, text)
	})
	router.GET("/surge/proxies", func(c *gin.Context) {
		proxyTypes := c.DefaultQuery("type", "")
		proxyCountry := c.DefaultQuery("c", "")
		proxyNotCountry := c.DefaultQuery("nc", "")
		proxySpeed := c.DefaultQuery("speed", "")
		proxyFilter := c.DefaultQuery("filter", "")
		text := ""
		if proxyTypes == "" && proxyCountry == "" && proxyNotCountry == "" && proxySpeed == "" {
			text = appcache.GetString("surgeproxies") // A string. To show speed in this if condition, this must be updated after speedtest
			if text == "" {
				proxies := appcache.GetProxies("proxies")
				surge := provider.Surge{
					Base: provider.Base{
						Proxies: &proxies,
					},
				}
				text = surge.Provide()
				appcache.SetString("surgeproxies", text)
			}
		} else if proxyTypes == "all" {
			proxies := appcache.GetProxies("allproxies")
			surge := provider.Surge{
				Base: provider.Base{
					Proxies:    &proxies,
					Types:      proxyTypes,
					Country:    proxyCountry,
					NotCountry: proxyNotCountry,
					Speed:      proxySpeed,
					Filter:     proxyFilter,
				},
			}
			text = surge.Provide()
		} else {
			proxies := appcache.GetProxies("proxies")
			surge := provider.Surge{
				Base: provider.Base{
					Proxies:    &proxies,
					Types:      proxyTypes,
					Country:    proxyCountry,
					NotCountry: proxyNotCountry,
					Filter:     proxyFilter,
				},
			}
			text = surge.Provide()
		}
		c.String(200, text)
	})
	router.GET("/forceupdate", func(c *gin.Context) {
		err := app.InitApp()
		if err != nil {
			c.String(http.StatusOK, err.Error())
		}
		c.String(http.StatusOK, "Updated")
	})

	router.GET("/api/proxies", func(c *gin.Context) {
		proxies := appcache.GetProxies("proxies")
		type proxyInfo struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			Port  int    `json:"port"`
			Delay int    `json:"delay"`
		}
		var list []proxyInfo
		for _, p := range proxies {
			info := proxyInfo{
				Name: p.BaseInfo().Name,
				Type: p.TypeName(),
				Port: p.BaseInfo().Port,
			}
			if stat, ok := healthcheck.ProxyStats.Find(p); ok {
				info.Delay = int(stat.Delay)
			}
			list = append(list, info)
		}
		c.JSON(http.StatusOK, list)
	})

	router.GET("/api/selected", func(c *gin.Context) {
		name := app.GetSelectedProxyName()
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"selected": false})
			return
		}
		proxies := appcache.GetProxies("proxies")
		var delay int
		for _, p := range proxies {
			if p.BaseInfo().Name == name {
				if stat, ok := healthcheck.ProxyStats.Find(p); ok {
					delay = int(stat.Delay)
				}
				break
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"selected": true,
			"name":     name,
			"delay":    delay,
		})
	})

	router.POST("/api/select", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		app.SelectProxyName(req.Name)

		meta, err := app.ResolveTargetForTest("www.gstatic.com", "80")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"name": req.Name, "status": "failed", "error": err.Error()})
			return
		}
		conn, err := app.DialSelectedProxyForTest(meta)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"name": req.Name, "status": "failed", "error": err.Error()})
			return
		}
		conn.Close()

		proxies := appcache.GetProxies("proxies")
		var delay int
		for _, p := range proxies {
			if p.BaseInfo().Name == req.Name {
				if stat, ok := healthcheck.ProxyStats.Find(p); ok {
					delay = int(stat.Delay)
				}
				break
			}
		}

		c.JSON(http.StatusOK, gin.H{"name": req.Name, "status": "connected", "delay": delay})
	})

	router.POST("/api/unselect", func(c *gin.Context) {
		app.UnselectProxy()
		c.JSON(http.StatusOK, gin.H{"selected": false})
	})
}

func Run() {
	setupRouter()
	servePort := config.Config.Port
	envp := os.Getenv("PORT") // envp for heroku. DO NOT SET ENV PORT IN PERSONAL SERVER UNLESS YOU KNOW WHAT YOU ARE DOING
	if envp != "" {
		servePort = envp
	}
	// Run on this server
	err := router.Run(":" + servePort)
	if err != nil {
		log.Fatalf("[router.go] Web server starting failed. Make sure your port %s has not been used. \n%s", servePort, err.Error())
	}
}

func injectDrawer(path string) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("injectDrawer: read %s error: %v", path, err)
		return
	}
	content := string(data)

	css := `<style>
.drawer-tab{position:fixed;top:50%;right:0;transform:translateY(-50%);z-index:999;cursor:pointer;background:#ff4532;color:#fff;padding:12px 6px;border-radius:6px 0 0 6px;writing-mode:vertical-lr;letter-spacing:2px;font-size:14px;user-select:none;transition:right .3s}
.drawer-tab.open{right:320px}
.drawer{position:fixed;top:0;right:-320px;width:320px;height:100%;z-index:1000;background:#1B1B1B;color:#fff;box-shadow:-2px 0 12px rgba(0,0,0,.5);transition:right .3s;display:flex;flex-direction:column}
.drawer.open{right:0}
.drawer-header{padding:16px 20px;font-size:18px;font-weight:700;border-bottom:1px solid #333;display:flex;justify-content:space-between;color:#ff4532}
.drawer-header .close-btn{cursor:pointer;font-size:20px;color:#666;border:none;background:none}
.drawer-header .close-btn:hover{color:#fff}
.drawer-body{padding:16px 20px;flex:1;overflow-y:auto}
.drawer-body label{display:block;margin-bottom:4px;font-size:13px;color:#aaa}
.drawer-body select{width:100%;padding:8px;font-size:14px;border:1px solid #444;border-radius:4px;margin-bottom:12px;box-sizing:border-box;background:#2a2a2a;color:#fff}
.drawer-body .btn-row{display:flex;gap:8px;margin-bottom:16px}
.drawer-body .btn-row button{flex:1;padding:8px;border:none;border-radius:4px;cursor:pointer;font-size:14px}
.btn-sel{background:#43ffe5;color:#1B1B1B;font-weight:700}
.btn-sel:disabled,.btn-uns:disabled{background:#444;color:#888;cursor:not-allowed}
.btn-uns{background:#ff4532;color:#fff;font-weight:700}
.btn-ref{background:#333;color:#dc7f7f;border:1px solid #444!important}
.st-box{padding:12px;border-radius:4px;margin:12px 0;font-size:14px;border:1px solid #333;background:#2a2a2a}
.st-box .lb{font-size:12px;color:#888;margin-bottom:4px}
.st-ok{color:#43ffe5;font-weight:700}
.st-fail{color:#ff4532;font-weight:700}
.st-idle{color:#666}
.ov{position:fixed;top:0;left:0;width:100%;height:100%;z-index:999;background:rgba(0,0,0,.5);display:none}
.ov.show{display:block}
</style>`

	html := `<div class="ov" id="ov" onclick="closeDrawer()"></div>
<div class="drawer-tab" id="dt" onclick="toggleDrawer()">代理 ▸</div>
<div class="drawer" id="dr">
<div class="drawer-header"><span>代理选择器</span><button class="close-btn" onclick="closeDrawer()">✕</button></div>
<div class="drawer-body">
<label>当前选中</label>
<div id="sel-info" class="st-box"><span class="st-idle">未选择</span></div>
<label>选择代理</label>
<select id="sel"><option value="">— 请选择 —</option></select>
<div class="btn-row"><button class="btn-sel" id="btn-sel" onclick="doSel()">选择</button><button class="btn-uns" id="btn-uns" onclick="doUnsel()">取消选择</button></div>
<div id="test-r" class="st-box" style="display:none"><div class="lb">连接测试</div><div id="test-t"></div></div>
<div class="btn-row"><button class="btn-ref" onclick="loadDrawer()">🔄 刷新列表</button></div>
<div style="font-size:12px;color:#999" id="dr-time"></div>
</div></div>
<script>
function toggleDrawer(){var d=document.getElementById('dr'),t=document.getElementById('dt'),o=document.getElementById('ov'),is=d.classList.toggle('open');t.classList.toggle('open');o.classList.toggle('show');t.textContent=is?'▸':'代理 ▸';if(is)loadDrawer()}
function closeDrawer(){document.getElementById('dr').classList.remove('open');document.getElementById('dt').classList.remove('open');document.getElementById('ov').classList.remove('show');document.getElementById('dt').textContent='代理 ▸'}
async function loadDrawer(){document.getElementById('dr-time').textContent='加载中...';try{var r=await Promise.all([fetch('/api/proxies').then(function(x){return x.json()}),fetch('/api/selected').then(function(x){return x.json()})]);renderDr(r[0],r[1])}catch(e){}document.getElementById('dr-time').textContent='更新于 '+new Date().toLocaleTimeString()}
function renderDr(px,sel){var s=document.getElementById('sel'),sn=sel&&sel.selected?sel.name:'';s.innerHTML='<option value="">— 请选择 —</option>'+(px||[]).map(function(p){return'<option value="'+eA(p.name)+'"'+(p.name===sn?' selected':'')+'>'+eH(p.name)+' ['+eH(p.type)+'] '+p.port+' '+(p.delay>0?p.delay+'ms':'-')+'</option>'}).join('');var d=document.getElementById('sel-info');if(!sel||!sel.selected){d.innerHTML='<span class="st-idle">未选择</span>'}else{d.innerHTML='<span class="st-ok">✅ '+eH(sel.name)+'</span>'+(sel.delay>0?' <span style="color:#aaa">('+sel.delay+'ms)</span>':'')}}
function doSel(){var s=document.getElementById('sel');if(!s.value)return;document.getElementById('btn-sel').disabled=true;document.getElementById('btn-uns').disabled=true;fetch('/api/select',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:s.value})}).then(function(r){return r.json()}).then(function(r){var tr=document.getElementById('test-r'),tt=document.getElementById('test-t');tr.style.display='block';tt.innerHTML=r.status==='connected'?'<span class="st-ok">✅ 已连接</span>'+(r.delay>0?' ('+r.delay+'ms)':''):'<span class="st-fail">❌ 连接失败</span>'+(r.error?': '+r.error:'');loadDrawer();document.getElementById('btn-sel').disabled=false;document.getElementById('btn-uns').disabled=false})}
function doUnsel(){document.getElementById('btn-sel').disabled=true;document.getElementById('btn-uns').disabled=true;fetch('/api/unselect',{method:'POST'}).then(function(){document.getElementById('test-r').style.display='none';document.getElementById('btn-sel').disabled=false;document.getElementById('btn-uns').disabled=false;loadDrawer()})}
function eH(s){if(!s)return'';return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;')}
function eA(s){if(!s)return'';return s.replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/'/g,'&#39;')}
</script>`

	if strings.Contains(content, "</head>") {
		content = strings.Replace(content, "</head>", css+"</head>", 1)
	}
	if strings.Contains(content, "</body>") {
		content = strings.Replace(content, "</body>", html+"</body>", 1)
	}

	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		log.Printf("injectDrawer: write %s error: %v", path, err)
	}
}

// 返回页面templates
func loadHTMLTemplate() (t *template.Template, err error) {
	t = template.New("")
	for _, fileName := range AssetNames() {
		if strings.Contains(fileName, "css") {
			continue
		}
		data := MustAsset(fileName)
		if _, statErr := os.Stat(fileName); statErr == nil {
			diskData, readErr := ioutil.ReadFile(fileName)
			if readErr == nil {
				data = diskData
			}
		}
		t, err = t.New(fileName).Parse(string(data))
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}
