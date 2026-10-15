package io.github.zzzhe2333.zzzmessage

import android.Manifest
import android.annotation.SuppressLint
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.View
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import org.json.JSONObject
import java.lang.ref.WeakReference
import java.net.URI

// Standalone Android shell with vendor-native push. The shared Vue chat UI
// is served from a self-hosted HTTPS web origin; backend selection is done in Vue.
class MainActivity : Activity() {
    private lateinit var input: EditText
    private lateinit var browser: WebView
    private lateinit var chooser: LinearLayout
    private var origin = ""

    companion object {
        private var running = WeakReference<MainActivity>(null)
        fun onRegistered(context: android.content.Context, provider: String, token: String) {
            PushState.save(context, provider, token)
            running.get()?.showNativeRegistration(provider, token)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        running = WeakReference(this)
        val parent = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(16, 24, 16, 8)
        }
        chooser = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        chooser.addView(TextView(this).apply {
            text = "ZzzMessage 私人聊天\n请输入你信任的聊天网站 HTTPS 地址"
            textSize = 19f
            setPadding(0, 0, 0, 12)
        })
        input = EditText(this).apply {
            hint = "https://chat.example.com"
            inputType = android.text.InputType.TYPE_TEXT_VARIATION_URI
            setSingleLine(true)
            setText(getPreferences(MODE_PRIVATE).getString("web_origin", "") ?: "")
        }
        chooser.addView(input)
        chooser.addView(Button(this).apply {
            text = "连接"
            setOnClickListener { openSite(input.text.toString()) }
        })
        chooser.addView(TextView(this).apply {
            text = "仅连接你信任的自建站点。聊天数据保存在本设备 WebView 数据区。\n厂商推送需官方 SDK、AppId 和服务端密钥。"
            setPadding(0, 12, 0, 0)
        })
        parent.addView(chooser)
        browser = WebView(this)
        parent.addView(browser, LinearLayout.LayoutParams(-1, 0, 1f))
        setContentView(parent)
        prepareWebView()
        val previous = getPreferences(MODE_PRIVATE).getString("web_origin", null)
        if (!previous.isNullOrBlank()) openSite(previous)
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun prepareWebView() {
        browser.settings.javaScriptEnabled = true
        browser.settings.domStorageEnabled = true
        browser.settings.allowFileAccess = false
        browser.settings.allowContentAccess = false
        browser.settings.mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_NEVER_ALLOW
        browser.settings.setSupportMultipleWindows(false)
        browser.webChromeClient = WebChromeClient()
        browser.addJavascriptInterface(Bridge(), "NativePushBridge")
        browser.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
                val u = request?.url ?: return true
                // Never expose the JavascriptInterface to a different website.
                return u.scheme != "https" || u.authority != URI(origin).rawAuthority
            }
            override fun onPageFinished(view: WebView?, url: String?) {
                if (url != null && url.startsWith(origin + "/")) {
                    PushState.get(this@MainActivity)?.let { showNativeRegistration(it.first, it.second) }
                }
            }
        }
    }

    private fun openSite(text: String) {
        val u = try { URI(text.trim()) } catch (_: Exception) { null }
        if (u == null || u.scheme != "https" || u.host.isNullOrBlank() || u.rawUserInfo != null ||
            u.rawQuery != null || u.rawFragment != null || u.port !in -1..65535) {
            input.error = "只允许有效的 HTTPS 域名或地址"
            return
        }
        origin = "https://" + u.rawAuthority
        getPreferences(MODE_PRIVATE).edit().putString("web_origin", origin).apply()
        chooser.visibility = View.GONE
        browser.visibility = View.VISIBLE
        browser.loadUrl(origin + "/")
    }

    fun showNativeRegistration(provider: String, token: String) {
        if (origin.isBlank()) return
        val payload = JSONObject().put("provider", provider).put("token", token).toString()
        runOnUiThread {
            browser.evaluateJavascript(
                "window.dispatchEvent(new CustomEvent('zzz:native-push', {detail:$payload}));", null
            )
        }
    }

    private inner class Bridge {
        @JavascriptInterface fun registration(): String =
            PushState.get(this@MainActivity)?.let {
                JSONObject().put("provider", it.first).put("token", it.second).toString()
            } ?: "{}"

        @JavascriptInterface fun manufacturer(): String =
            Build.MANUFACTURER.lowercase(java.util.Locale.ROOT)

        @JavascriptInterface fun enable() {
            runOnUiThread {
                if (Build.VERSION.SDK_INT >= 33 &&
                    checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                    requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 700)
                } else {
                    registerVendor()
                }
            }
        }
        @JavascriptInterface fun disable() {
            runOnUiThread { PushState.unregister(this@MainActivity) }
        }
    }

    @Deprecated("Platform permission callback")
    override fun onRequestPermissionsResult(code: Int, permissions: Array<out String>, grants: IntArray) {
        super.onRequestPermissionsResult(code, permissions, grants)
        if (code == 700 && grants.isNotEmpty() && grants[0] == PackageManager.PERMISSION_GRANTED) {
            registerVendor()
        }
    }

    private fun registerVendor() {
        try {
            PushState.register(this)
        } catch (e: Exception) {
            val details = JSONObject.quote(e.message ?: "厂商推送初始化失败")
            browser.evaluateJavascript(
                "window.dispatchEvent(new CustomEvent('zzz:native-push-error', {detail:$details}));", null
            )
        }
    }

    override fun onBackPressed() {
        if (browser.visibility == View.VISIBLE && browser.canGoBack()) browser.goBack()
        else if (browser.visibility == View.VISIBLE) {
            browser.visibility = View.GONE
            chooser.visibility = View.VISIBLE
            origin = ""
        } else super.onBackPressed()
    }

    override fun onDestroy() {
        if (running.get() === this) running.clear()
        browser.removeJavascriptInterface("NativePushBridge")
        browser.destroy()
        super.onDestroy()
    }
}
