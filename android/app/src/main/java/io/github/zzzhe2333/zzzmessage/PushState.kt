package io.github.zzzhe2333.zzzmessage

import android.content.Context
import android.os.Build
import com.vivo.push.IPushActionListener
import com.vivo.push.PushClient
import com.xiaomi.mipush.sdk.MiPushClient
import java.util.Locale

object PushState {
    private const val NAME = "vendor_push"
    private fun prefs(context: Context) = context.getSharedPreferences(NAME, Context.MODE_PRIVATE)

    fun save(context: Context, provider: String, token: String) {
        if (token.isBlank()) return
        prefs(context).edit().putString("provider", provider).putString("token", token).apply()
    }
    fun get(context: Context): Pair<String, String>? {
        val p = prefs(context).getString("provider", null) ?: return null
        val token = prefs(context).getString("token", null) ?: return null
        return p to token
    }

    // IMPORTANT: only called after a user's explicit opt-in in the Vue settings.
    // Vivo requires privacy notice/consent BEFORE SDK initialization.
    fun register(activity: MainActivity) {
        when (Build.MANUFACTURER.lowercase(Locale.ROOT)) {
            "xiaomi", "redmi" -> {
                check(BuildConfig.XIAOMI_APP_ID.isNotBlank() && BuildConfig.XIAOMI_APP_KEY.isNotBlank()) {
                    "未设置小米 AppId/AppKey"
                }
                MiPushClient.registerPush(activity.applicationContext, BuildConfig.XIAOMI_APP_ID, BuildConfig.XIAOMI_APP_KEY)
                MiPushClient.getRegId(activity)?.takeIf { it.isNotBlank() }?.let {
                    MainActivity.onRegistered(activity, "xiaomi", it)
                }
            }
            "vivo", "iqoo" -> {
                val client = PushClient.getInstance(activity.applicationContext)
                client.initialize()
                client.turnOnPush(object : IPushActionListener {
                    override fun onStateChanged(state: Int) {
                        if (state == 0) {
                            val token = client.regId
                            if (!token.isNullOrBlank()) {
                                MainActivity.onRegistered(activity, "vivo", token)
                            }
                        }
                    }
                })
            }
            else -> throw IllegalStateException("首批仅支持 Xiaomi/Redmi 和 vivo/iQOO")
        }
    }

    fun unregister(context: Context) {
        when (get(context)?.first) {
            "xiaomi" -> MiPushClient.unregisterPush(context)
            "vivo" -> PushClient.getInstance(context).turnOffPush(object : IPushActionListener {
                override fun onStateChanged(state: Int) {}
            })
        }
        prefs(context).edit().clear().apply()
    }
}
