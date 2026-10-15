import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val local = Properties().apply {
    val props = rootProject.file("local.properties")
    if (props.isFile) props.inputStream().use { load(it) }
}
fun clientValue(key: String): String =
    (local.getProperty(key) ?: System.getenv(key) ?: "").replace("\\", "\\\\").replace("\"", "\\\"")

android {
    namespace = "io.github.zzzhe2333.zzzmessage"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.zzzhe2333.zzzmessage"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.2.0"

        buildConfigField("String", "XIAOMI_APP_ID", "\"${clientValue("XIAOMI_APP_ID")}\"")
        buildConfigField("String", "XIAOMI_APP_KEY", "\"${clientValue("XIAOMI_APP_KEY")}\"")
        manifestPlaceholders["vivoAppId"] = clientValue("VIVO_APP_ID")
        manifestPlaceholders["vivoAppKey"] = clientValue("VIVO_APP_KEY")
    }

    buildFeatures { buildConfig = true }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    packaging { resources { excludes += "/META-INF/{AL2.0,LGPL2.1}" } }
}

dependencies {
    // Obtain official Xiaomi MiPush and vivo Push AARs via vendor portals.
    // Never redistribute proprietary SDK binaries in this open-source repo.
    implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar", "*.jar"))))
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.activity:activity-ktx:1.9.3")
}
