package life.integ.familydaily;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

public class LanguageSettingsTest {
    @Test
    public void defaultsUnknownAndMissingValuesToEnglish() {
        assertEquals("en", LanguageSettings.normalize(null));
        assertEquals("en", LanguageSettings.normalize(""));
        assertEquals("en", LanguageSettings.normalize("fr"));
    }

    @Test
    public void keepsSupportedLanguageCodes() {
        assertEquals("en", LanguageSettings.normalize("en"));
        assertEquals("zh", LanguageSettings.normalize(" ZH "));
        assertEquals("Hello", LanguageSettings.text("en", "Hello", "你好"));
        assertEquals("你好", LanguageSettings.text("zh", "Hello", "你好"));
    }
}
