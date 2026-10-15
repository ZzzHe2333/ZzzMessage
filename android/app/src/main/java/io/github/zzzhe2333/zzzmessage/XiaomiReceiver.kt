package io.github.zzzhe2333.zzzmessage

import android.content.Context
import com.xiaomi.mipush.sdk.MiPushClient
import com.xiaomi.mipush.sdk.MiPushCommandMessage
import com.xiaomi.mipush.sdk.PushMessageReceiver

class XiaomiReceiver : PushMessageReceiver() {
    override fun onReceiveRegisterResult(context: Context, message: MiPushCommandMessage) {
        if (message.command == MiPushClient.COMMAND_REGISTER && message.resultCode == 0L) {
            val token = message.commandArguments?.firstOrNull()
            if (!token.isNullOrBlank()) {
                MainActivity.onRegistered(context, "xiaomi", token)
            }
        }
    }
    override fun onCommandResult(context: Context, message: MiPushCommandMessage) {
        onReceiveRegisterResult(context, message)
    }
}
