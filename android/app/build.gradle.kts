plugins {
    id("com.android.application")
}

android {
    namespace = "life.integ.familydaily"
    compileSdk = 37

    defaultConfig {
        applicationId = "life.integ.familydaily"
        minSdk = 26
        targetSdk = 37
        versionCode = 1
        versionName = "0.1.0"

        buildConfigField("String", "API_BASE_URL", "\"http://10.0.2.2:8080\"")
        buildConfigField("String", "FAMILY_API_TOKEN", "\"family-daily-local\"")
    }

    buildFeatures {
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

