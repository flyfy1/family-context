package life.integ.familydaily;

import android.Manifest;
import android.app.AlertDialog;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.media.MediaPlayer;
import android.media.MediaRecorder;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends android.app.Activity {
    private static final int RECORD_AUDIO_REQUEST = 42;
    private static final int PHOTO_ACCESS_REQUEST = 43;
    private static final int COLOR_BACKGROUND = Color.rgb(255, 249, 244);
    private static final int COLOR_CARD = Color.WHITE;
    private static final int COLOR_PRIMARY = Color.rgb(185, 79, 50);
    private static final int COLOR_TEXT = Color.rgb(48, 43, 39);
    private static final int COLOR_MUTED = Color.rgb(113, 102, 94);

    private final ExecutorService executor = Executors.newSingleThreadExecutor();
    private LinearLayout feedContainer;
    private EditText questionInput;
    private ProgressBar progress;
    private TextView status;
    private TextView photoSyncStatus;
    private Button photoSyncButton;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private MediaRecorder recorder;
    private File recordingFile;
    private JSONObject pendingRecordQuestion;
    private String language;
    private MemberProfileSettings.Profile profile;
    private JSONArray familyMembers = new JSONArray();
    private Button profileButton;
    private LinearLayout storyContainer;
    private TextView storyChildLabel;
    private String storyChildID = "";
    private TextView dailySummaryText;
    private Button storyGenerateButton;
    private Button pendingDailyRecordButton;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        language = LanguageSettings.get(this);
        profile = MemberProfileSettings.get(this);
        setContentView(buildScreen());
        loadMembers();
        if (MemberProfileSettings.ELDER.equals(profile.role)) loadDailySummary();
        else if (!MemberProfileSettings.CHILD.equals(profile.role)) loadFeed();
        PhotoSync.schedule(this);
        refreshPhotoSyncStatus();
    }

    @Override
    protected void onResume() {
        super.onResume();
        refreshPhotoSyncStatus();
        if (PhotoSync.preferences(this).getBoolean(PhotoSync.KEY_ENABLED, false)
                && PhotoSync.isConfigured(this) && PhotoSync.hasImageAccess(this)) {
            PhotoSync.syncNow(this);
            pollPhotoSyncStatus(0);
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        releaseRecorder();
        executor.shutdownNow();
        handler.removeCallbacksAndMessages(null);
    }

    private View buildScreen() {
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(COLOR_BACKGROUND);

        LinearLayout root = vertical();
        root.setPadding(dp(20), dp(28), dp(20), dp(40));
        scroll.addView(root, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        TextView eyebrow = text("FAMILY DAILY · V1", 13, COLOR_PRIMARY);
        root.addView(eyebrow);
        String titleText = MemberProfileSettings.ELDER.equals(profile.role)
                ? tr("How was your day?", "今天过得怎么样？")
                : MemberProfileSettings.CHILD.equals(profile.role)
                ? tr("A story made from\nyour family's day", "把家里今天的事\n变成一个故事")
                : tr("Turn one caring question\ninto a conversation", "把一句关心，\n变成一次对话");
        TextView title = text(titleText, 30, COLOR_TEXT);
        title.setPadding(0, dp(8), 0, dp(8));
        root.addView(title);
        String introText = MemberProfileSettings.ELDER.equals(profile.role)
                ? tr("Tap once to share your day by voice, then listen to the family daily.", "点一下就能用语音说说今天，再听听家里的日报。")
                : MemberProfileSettings.CHILD.equals(profile.role)
                ? tr("A grown-up can make a bedtime story for you from moments the family shared.", "家人可以把大家分享的日常，变成一个讲给你的睡前故事。")
                : tr("Ask Dad one specific question. He can answer by voice with a single tap.", "先问爸爸一个具体的问题。他只需要点一下按钮，就能用语音回答。");
        TextView intro = text(introText, 16, COLOR_MUTED);
        intro.setLineSpacing(0, 1.25f);
        root.addView(intro);

        LinearLayout languageCard = card();
        languageCard.setPadding(dp(18), dp(18), dp(18), dp(18));
        LinearLayout.LayoutParams languageCardParams = fullWidth();
        languageCardParams.setMargins(0, dp(24), 0, 0);
        root.addView(languageCard, languageCardParams);
        languageCard.addView(text(tr("Settings", "设置"), 20, COLOR_TEXT));
        TextView profileIntro = text(tr("Current family profile", "当前家庭身份"), 13, COLOR_MUTED);
        profileIntro.setPadding(0, dp(8), 0, dp(8));
        languageCard.addView(profileIntro);
        profileButton = secondaryButton(profileLabel(profile));
        profileButton.setContentDescription(tr("Choose family profile", "选择家庭身份"));
        profileButton.setOnClickListener(v -> showProfileSettings());
        languageCard.addView(profileButton, fullWidth());
        TextView languageIntro = text(tr("Language", "语言"), 13, COLOR_MUTED);
        languageIntro.setPadding(0, dp(8), 0, dp(8));
        languageCard.addView(languageIntro);
        Button languageButton = secondaryButton(LanguageSettings.CHINESE.equals(language) ? "中文" : "English");
        languageButton.setContentDescription(tr("Choose app language", "选择 App 语言"));
        languageButton.setOnClickListener(v -> showLanguageSettings());
        languageCard.addView(languageButton, fullWidth());

        if (MemberProfileSettings.ELDER.equals(profile.role)) {
            addElderMode(root);
            addStatusArea(root);
            return scroll;
        }
        if (MemberProfileSettings.CHILD.equals(profile.role)) {
            addStoryCard(root);
            addStatusArea(root);
            return scroll;
        }

        LinearLayout syncCard = card();
        syncCard.setPadding(dp(18), dp(18), dp(18), dp(18));
        LinearLayout.LayoutParams syncCardParams = fullWidth();
        syncCardParams.setMargins(0, dp(24), 0, 0);
        root.addView(syncCard, syncCardParams);
        syncCard.addView(text(tr("Automatic photo backup", "照片自动备份"), 20, COLOR_TEXT));
        TextView syncIntro = text(tr("After permission is granted, existing and new photos sync to your private NAS Space when the app opens and on a system schedule.", "授权后，现有和新照片会在打开 App 时补同步，并由系统定期同步到你的私人 NAS 空间。"), 14, COLOR_MUTED);
        syncIntro.setLineSpacing(0, 1.2f);
        syncIntro.setPadding(0, dp(8), 0, dp(12));
        syncCard.addView(syncIntro);
        photoSyncStatus = text(tr("Not configured", "尚未配置"), 14, COLOR_MUTED);
        photoSyncStatus.setPadding(0, 0, 0, dp(10));
        syncCard.addView(photoSyncStatus);
        LinearLayout syncActions = new LinearLayout(this);
        Button configure = secondaryButton(tr("Connect NAS", "连接 NAS"));
        configure.setOnClickListener(v -> showPhotoSyncSettings());
        syncActions.addView(configure, new LinearLayout.LayoutParams(0, dp(52), 1));
        photoSyncButton = primaryButton(tr("Enable sync", "开启同步"));
        LinearLayout.LayoutParams syncButtonParams = new LinearLayout.LayoutParams(0, dp(52), 1);
        syncButtonParams.setMargins(dp(10), 0, 0, 0);
        photoSyncButton.setOnClickListener(v -> enableOrSyncPhotos());
        syncActions.addView(photoSyncButton, syncButtonParams);
        syncCard.addView(syncActions, fullWidth());

        addStoryCard(root);

        LinearLayout askCard = card();
        askCard.setPadding(dp(18), dp(18), dp(18), dp(18));
        LinearLayout.LayoutParams cardParams = fullWidth();
        cardParams.setMargins(0, dp(24), 0, dp(22));
        root.addView(askCard, cardParams);
        askCard.addView(text(tr("Ask Dad a question", "问爸爸一个问题"), 20, COLOR_TEXT));

        questionInput = new EditText(this);
        questionInput.setHint(tr("For example: Did your friend make it to the doctor?", "例如：老张后来去医院了吗？"));
        questionInput.setTextSize(16);
        questionInput.setMinHeight(dp(54));
        questionInput.setPadding(dp(14), dp(10), dp(14), dp(10));
        questionInput.setSingleLine(false);
        questionInput.setMaxLines(3);
        questionInput.setBackground(rounded(Color.rgb(248, 242, 237), dp(12)));
        LinearLayout.LayoutParams inputParams = fullWidth();
        inputParams.setMargins(0, dp(14), 0, dp(12));
        askCard.addView(questionInput, inputParams);

        Button askButton = primaryButton(tr("Send question", "发送问题"));
        askButton.setOnClickListener(v -> createQuestion());
        askCard.addView(askButton, fullWidth());

        LinearLayout heading = new LinearLayout(this);
        heading.setGravity(Gravity.CENTER_VERTICAL);
        heading.addView(text(tr("Family activity", "家庭动态"), 22, COLOR_TEXT), new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1));
        Button refresh = secondaryButton(tr("Refresh", "刷新"));
        refresh.setOnClickListener(v -> loadFeed());
        heading.addView(refresh);
        root.addView(heading, fullWidth());

        addStatusArea(root);
        return scroll;
    }

    private void addStatusArea(LinearLayout root) {
        progress = new ProgressBar(this);
        progress.setVisibility(View.GONE);
        LinearLayout.LayoutParams progressParams = new LinearLayout.LayoutParams(dp(28), dp(28));
        progressParams.gravity = Gravity.CENTER_HORIZONTAL;
        progressParams.setMargins(0, dp(16), 0, dp(8));
        root.addView(progress, progressParams);
        status = text("", 14, COLOR_MUTED);
        status.setGravity(Gravity.CENTER);
        status.setPadding(0, dp(12), 0, dp(4));
        status.setVisibility(View.GONE);
        root.addView(status, fullWidth());
        feedContainer = vertical();
        root.addView(feedContainer, fullWidth());
    }

    private void addElderMode(LinearLayout root) {
        LinearLayout voiceCard = card();
        voiceCard.setPadding(dp(18), dp(22), dp(18), dp(22));
        LinearLayout.LayoutParams params = fullWidth();
        params.setMargins(0, dp(24), 0, 0);
        root.addView(voiceCard, params);
        voiceCard.addView(text(tr("Tell the family about today", "说说今天的事"), 22, COLOR_TEXT));
        TextView help = text(tr("Your recording will be organized and shared with the family.", "录音会自动整理，并分享给家人。"), 15, COLOR_MUTED);
        help.setPadding(0, dp(8), 0, dp(14));
        voiceCard.addView(help);
        Button record = primaryButton(tr("●  Start speaking", "●  开始说话"));
        record.setMinHeight(dp(76));
        record.setTextSize(20);
        record.setOnClickListener(v -> beginDailyRecording(record));
        voiceCard.addView(record, fullWidth());

        LinearLayout dailyCard = card();
        dailyCard.setPadding(dp(18), dp(22), dp(18), dp(22));
        LinearLayout.LayoutParams dailyParams = fullWidth();
        dailyParams.setMargins(0, dp(24), 0, 0);
        root.addView(dailyCard, dailyParams);
        dailyCard.addView(text(tr("Our family today", "我们家今天"), 22, COLOR_TEXT));
        dailySummaryText = text(tr("Loading the family daily…", "正在读取家庭日报……"), 17, COLOR_MUTED);
        dailySummaryText.setLineSpacing(0, 1.3f);
        dailySummaryText.setPadding(0, dp(12), 0, dp(14));
        dailyCard.addView(dailySummaryText);
        Button refresh = secondaryButton(tr("Refresh family daily", "刷新家庭日报"));
        refresh.setOnClickListener(v -> loadDailySummary());
        dailyCard.addView(refresh, fullWidth());
    }

    private void addStoryCard(LinearLayout root) {
        LinearLayout storyCard = card();
        storyCard.setPadding(dp(18), dp(20), dp(18), dp(20));
        LinearLayout.LayoutParams params = fullWidth();
        params.setMargins(0, dp(24), 0, 0);
        root.addView(storyCard, params);
        storyCard.addView(text(tr("A bedtime story for a child", "给孩子的睡前故事"), 22, COLOR_TEXT));
        TextView intro = text(tr("Any family member can make a story for a child from family-visible moments.", "任何家庭成员都可以把全家可见的日常，生成讲给孩子的故事。"), 14, COLOR_MUTED);
        intro.setPadding(0, dp(8), 0, dp(12));
        storyCard.addView(intro);
        storyChildLabel = text(tr("Choose a child after family profiles load", "家庭身份加载后请选择孩子"), 14, COLOR_MUTED);
        storyChildLabel.setPadding(0, 0, 0, dp(10));
        storyCard.addView(storyChildLabel);
        Button choose = secondaryButton(tr("Choose child", "选择孩子"));
        choose.setOnClickListener(v -> showStoryChildChooser());
        storyCard.addView(choose, fullWidth());
        storyGenerateButton = primaryButton(tr("Generate story", "生成故事"));
        storyGenerateButton.setEnabled(false);
        LinearLayout.LayoutParams buttonParams = fullWidth();
        buttonParams.setMargins(0, dp(10), 0, dp(10));
        storyGenerateButton.setOnClickListener(v -> generateStory());
        storyCard.addView(storyGenerateButton, buttonParams);
        storyContainer = vertical();
        storyCard.addView(storyContainer, fullWidth());
    }

    private void loadMembers() {
        executor.execute(() -> {
            try {
                JSONArray members = new JSONObject(request("GET", "/api/v1/members", null)).getJSONArray("members");
                JSONObject selected = null;
                for (int i = 0; i < members.length(); i++) {
                    JSONObject member = members.optJSONObject(i);
                    if (member != null && member.optString("id").equals(profile.id)) selected = member;
                }
                if (selected == null && members.length() > 0) selected = members.optJSONObject(0);
                JSONObject finalSelected = selected;
                runOnUiThread(() -> {
                    familyMembers = members;
                    if (finalSelected != null && (!finalSelected.optString("id").equals(profile.id)
                            || !finalSelected.optString("role").equals(profile.role)
                            || !finalSelected.optString("name").equals(profile.name))) {
                        MemberProfileSettings.set(this, finalSelected.optString("id"), finalSelected.optString("name"), finalSelected.optString("role"));
                        recreate();
                        return;
                    }
                    if (profileButton != null) profileButton.setText(profileLabel(profile));
                    selectDefaultStoryChild();
                });
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void showProfileSettings() {
        if (recorder != null) {
            Toast.makeText(this, tr("Finish the recording before switching profile", "请先结束录音，再切换身份"), Toast.LENGTH_LONG).show();
            return;
        }
        if (familyMembers.length() == 0) {
            Toast.makeText(this, tr("Family profiles are still loading", "家庭身份仍在加载"), Toast.LENGTH_SHORT).show();
            return;
        }
        String[] labels = new String[familyMembers.length()];
        int selected = 0;
        for (int i = 0; i < familyMembers.length(); i++) {
            JSONObject member = familyMembers.optJSONObject(i);
            labels[i] = member.optString("name") + " · " + roleLabel(member.optString("role"));
            if (member.optString("id").equals(profile.id)) selected = i;
        }
        new AlertDialog.Builder(this)
                .setTitle(tr("Who is using this phone?", "谁正在使用这台手机？"))
                .setSingleChoiceItems(labels, selected, (dialog, which) -> {
                    JSONObject member = familyMembers.optJSONObject(which);
                    if (member != null && !member.optString("id").equals(profile.id)) {
                        MemberProfileSettings.set(this, member.optString("id"), member.optString("name"), member.optString("role"));
                        dialog.dismiss();
                        recreate();
                    } else dialog.dismiss();
                })
                .setNegativeButton(tr("Cancel", "取消"), null)
                .show();
    }

    private String profileLabel(MemberProfileSettings.Profile value) {
        if (value == null || value.id.isEmpty()) return tr("Choose profile", "选择身份");
        return value.name + " · " + roleLabel(value.role);
    }

    private String roleLabel(String role) {
        if (MemberProfileSettings.ELDER.equals(role)) return tr("Elder mode", "老人模式");
        if (MemberProfileSettings.CHILD.equals(role)) return tr("Child mode", "孩子模式");
        return tr("Standard mode", "普通模式");
    }

    private void selectDefaultStoryChild() {
        JSONObject selected = null;
        for (int i = 0; i < familyMembers.length(); i++) {
            JSONObject member = familyMembers.optJSONObject(i);
            if (member != null && MemberProfileSettings.CHILD.equals(member.optString("role"))) {
                if (member.optString("id").equals(storyChildID) || member.optString("id").equals(profile.id)) {
                    selected = member;
                    break;
                }
                if (selected == null) selected = member;
            }
        }
        if (selected == null) {
            storyChildID = "";
            if (storyChildLabel != null) storyChildLabel.setText(tr("No child profile is configured", "还没有配置孩子身份"));
            if (storyGenerateButton != null) storyGenerateButton.setEnabled(false);
            return;
        }
        storyChildID = selected.optString("id");
        if (storyChildLabel != null) storyChildLabel.setText(tr("Story for ", "故事讲给：") + selected.optString("name"));
        if (storyGenerateButton != null) storyGenerateButton.setEnabled(true);
        loadStories();
    }

    private void showStoryChildChooser() {
        JSONArray children = new JSONArray();
        for (int i = 0; i < familyMembers.length(); i++) {
            JSONObject member = familyMembers.optJSONObject(i);
            if (member != null && MemberProfileSettings.CHILD.equals(member.optString("role"))) children.put(member);
        }
        if (children.length() == 0) {
            Toast.makeText(this, tr("Ask the family administrator to mark a member as a child", "请让家庭管理员把成员标记为孩子"), Toast.LENGTH_LONG).show();
            return;
        }
        String[] labels = new String[children.length()];
        int selected = 0;
        for (int i = 0; i < children.length(); i++) {
            JSONObject child = children.optJSONObject(i);
            labels[i] = child.optString("name");
            if (child.optString("id").equals(storyChildID)) selected = i;
        }
        new AlertDialog.Builder(this)
                .setTitle(tr("Choose a child", "选择孩子"))
                .setSingleChoiceItems(labels, selected, (dialog, which) -> {
                    storyChildID = children.optJSONObject(which).optString("id");
                    dialog.dismiss();
                    selectDefaultStoryChild();
                })
                .setNegativeButton(tr("Cancel", "取消"), null)
                .show();
    }

    private void generateStory() {
        if (storyChildID.isEmpty()) return;
        storyGenerateButton.setEnabled(false);
        storyGenerateButton.setText(tr("Generating story and audio…", "正在生成故事和音频……"));
        executor.execute(() -> {
            try {
                JSONObject body = new JSONObject()
                        .put("familyId", "our-family")
                        .put("childId", storyChildID)
                        .put("audienceAge", 6)
                        .put("days", 7)
                        .put("language", language);
                JSONObject story = new JSONObject(request("POST", "/api/v1/bedtime-stories", body));
                runOnUiThread(() -> {
                    renderStory(story);
                    storyGenerateButton.setEnabled(true);
                    storyGenerateButton.setText(tr("Generate another story", "再生成一个故事"));
                });
            } catch (Exception error) {
                runOnUiThread(() -> {
                    storyGenerateButton.setEnabled(true);
                    storyGenerateButton.setText(tr("Generate story", "生成故事"));
                });
                showError(error);
            }
        });
    }

    private void loadStories() {
        if (storyContainer == null || storyChildID.isEmpty()) return;
        executor.execute(() -> {
            try {
                JSONObject response = new JSONObject(request("GET", "/api/v1/bedtime-stories?childId=" + storyChildID + "&language=" + language, null));
                JSONArray stories = response.optJSONArray("bedtimeStories");
                JSONObject latest = stories != null && stories.length() > 0 ? stories.optJSONObject(0) : null;
                runOnUiThread(() -> {
                    if (latest == null) {
                        storyContainer.removeAllViews();
                        storyContainer.addView(text(tr("No story yet. Generate the first one.", "还没有故事，可以生成第一个。"), 14, COLOR_MUTED));
                    } else renderStory(latest);
                });
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void renderStory(JSONObject story) {
        if (storyContainer == null) return;
        storyContainer.removeAllViews();
        TextView title = text(story.optString("title"), 20, COLOR_TEXT);
        title.setPadding(0, dp(10), 0, dp(8));
        storyContainer.addView(title);
        TextView content = text(story.optString("content"), 16, COLOR_TEXT);
        content.setLineSpacing(0, 1.3f);
        storyContainer.addView(content);
        String audioURL = story.optString("audioUrl");
        if (!audioURL.isEmpty()) {
            Button play = secondaryButton(tr("▶  Play bedtime story", "▶  播放睡前故事"));
            LinearLayout.LayoutParams params = fullWidth();
            params.setMargins(0, dp(12), 0, 0);
            play.setOnClickListener(v -> playAudio(
                    audioURL,
                    play,
                    tr("▶  Play bedtime story", "▶  播放睡前故事")
            ));
            storyContainer.addView(play, params);
        } else {
            TextView state = text(tr("The story text is ready; audio is still unavailable.", "故事文字已经准备好，音频暂时不可用。"), 13, COLOR_MUTED);
            state.setPadding(0, dp(10), 0, 0);
            storyContainer.addView(state);
        }
    }

    private void loadDailySummary() {
        executor.execute(() -> {
            try {
                JSONObject response = new JSONObject(request("GET", "/api/v1/daily-summaries/latest?language=" + language, null));
                JSONObject summary = response.optJSONObject("summary");
                runOnUiThread(() -> {
                    if (dailySummaryText != null) dailySummaryText.setText(summary == null
                            ? tr("Today's family daily has not been generated yet.", "今天的家庭日报还没有生成。")
                            : summary.optString("content"));
                });
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void beginDailyRecording(Button button) {
        if (profile.id.isEmpty()) {
            Toast.makeText(this, tr("Choose a family profile first", "请先选择家庭身份"), Toast.LENGTH_LONG).show();
            return;
        }
        pendingDailyRecordButton = button;
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO}, RECORD_AUDIO_REQUEST);
            return;
        }
        startDailyRecording(button);
    }

    private void startDailyRecording(Button button) {
        try {
            recordingFile = new File(getCacheDir(), "daily-" + UUID.randomUUID() + ".m4a");
            recorder = Build.VERSION.SDK_INT >= 31 ? new MediaRecorder(this) : new MediaRecorder();
            recorder.setAudioSource(MediaRecorder.AudioSource.MIC);
            recorder.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4);
            recorder.setAudioEncoder(MediaRecorder.AudioEncoder.AAC);
            recorder.setAudioEncodingBitRate(64_000);
            recorder.setAudioSamplingRate(44_100);
            recorder.setOutputFile(recordingFile.getAbsolutePath());
            recorder.prepare();
            recorder.start();
            button.setText(tr("■  Stop and share", "■  结束并分享"));
            button.setOnClickListener(v -> stopAndUploadDaily(button));
            Toast.makeText(this, tr("Recording — speak naturally", "正在录音，请自然地说话"), Toast.LENGTH_SHORT).show();
        } catch (Exception error) {
            releaseRecorder();
            Toast.makeText(this, tr("Unable to start recording", "暂时无法开始录音"), Toast.LENGTH_LONG).show();
        }
    }

    private void stopAndUploadDaily(Button button) {
        try {
            recorder.stop();
        } catch (RuntimeException tooShort) {
            releaseRecorder();
            if (recordingFile != null) recordingFile.delete();
            button.setText(tr("●  Start speaking", "●  开始说话"));
            button.setOnClickListener(v -> beginDailyRecording(button));
            Toast.makeText(this, tr("The recording was too short. Please try again.", "录音太短，请再说一次"), Toast.LENGTH_LONG).show();
            return;
        }
        releaseRecorder();
        File file = recordingFile;
        setBusy(true, tr("Organizing and sharing your update…", "正在整理并分享你的近况……"));
        executor.execute(() -> {
            try {
                uploadDailyVoice(file);
                if (file != null) file.delete();
                runOnUiThread(() -> {
                    button.setText(tr("●  Share another update", "●  再说一条"));
                    button.setOnClickListener(v -> beginDailyRecording(button));
                    setBusy(false, tr("Your update was shared with the family", "你的近况已经分享给家人"));
                });
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void uploadDailyVoice(File audio) throws Exception {
        String boundary = "FamilyDaily-" + UUID.randomUUID();
        HttpURLConnection connection = openConnection("POST", "/api/v1/updates/voice");
        connection.setRequestProperty("Content-Type", "multipart/form-data; boundary=" + boundary);
        connection.setDoOutput(true);
        try (OutputStream output = new BufferedOutputStream(connection.getOutputStream())) {
            writeUtf8(output, "--" + boundary + "\r\nContent-Disposition: form-data; name=\"familyId\"\r\n\r\nour-family\r\n");
            writeUtf8(output, "--" + boundary + "\r\nContent-Disposition: form-data; name=\"memberId\"\r\n\r\n" + profile.id + "\r\n");
            writeUtf8(output, "--" + boundary + "\r\nContent-Disposition: form-data; name=\"visibility\"\r\n\r\nfamily\r\n");
            writeUtf8(output, "--" + boundary + "\r\nContent-Disposition: form-data; name=\"audio\"; filename=\"daily.m4a\"\r\nContent-Type: audio/mp4\r\n\r\n");
            try (InputStream input = new BufferedInputStream(new FileInputStream(audio))) {
                byte[] buffer = new byte[8192];
                int read;
                while ((read = input.read(buffer)) != -1) output.write(buffer, 0, read);
            }
            writeUtf8(output, "\r\n--" + boundary + "--\r\n");
        }
        readResponse(connection);
    }

    private void createQuestion() {
        String text = questionInput.getText().toString().trim();
        if (text.isEmpty()) {
            questionInput.setError(tr("Write the question you want to ask", "先写下你想问的问题"));
            return;
        }
        setBusy(true, tr("Sending question…", "正在发送问题……"));
        executor.execute(() -> {
            try {
                JSONObject body = new JSONObject()
                        .put("familyId", "our-family")
                        .put("askedBy", "洋宇")
                        .put("askedTo", "爸爸")
                        .put("text", text);
                request("POST", "/api/v1/questions", body);
                runOnUiThread(() -> {
                    questionInput.setText("");
                    Toast.makeText(this, tr("Your question is ready", "问题已经准备好"), Toast.LENGTH_SHORT).show();
                });
                loadFeedInBackground();
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void loadFeed() {
        setBusy(true, tr("Loading family activity…", "正在读取家庭动态……"));
        executor.execute(this::loadFeedInBackground);
    }

    private void loadFeedInBackground() {
        try {
            JSONObject response = new JSONObject(request("GET", "/api/v1/questions", null));
            JSONArray questions = response.getJSONArray("questions");
            runOnUiThread(() -> renderFeed(questions));
        } catch (Exception error) {
            showError(error);
        }
    }

    private void renderFeed(JSONArray questions) {
        setBusy(false, questions.length() == 0 ? tr("No questions yet. Start with one specific thing you care about.", "还没有问题。先从一句具体的关心开始吧。") : "");
        feedContainer.removeAllViews();
        for (int i = 0; i < questions.length(); i++) {
            JSONObject question = questions.optJSONObject(i);
            if (question != null) {
                feedContainer.addView(buildQuestionCard(question), cardSpacing());
            }
        }
    }

    private View buildQuestionCard(JSONObject question) {
        LinearLayout card = card();
        card.setPadding(dp(18), dp(18), dp(18), dp(18));
        TextView meta = text(tr(question.optString("askedBy") + " wants to ask " + question.optString("askedTo"), question.optString("askedBy") + " 想问 " + question.optString("askedTo")), 13, COLOR_PRIMARY);
        card.addView(meta);
        TextView questionText = text(question.optString("text"), 20, COLOR_TEXT);
        questionText.setLineSpacing(0, 1.18f);
        questionText.setPadding(0, dp(8), 0, dp(14));
        card.addView(questionText);

        JSONObject answer = question.optJSONObject("answer");
        if (answer == null) {
            Button record = primaryButton(tr("🎙  Answer this question", "🎙  回答这个问题"));
            record.setMinHeight(dp(58));
            record.setTextSize(18);
            record.setOnClickListener(v -> beginRecording(question, record));
            card.addView(record, fullWidth());
            return card;
        }

        String answerStatus = answer.optString("status");
        if ("processing_failed".equals(answerStatus)) {
            card.addView(infoBox(answer.optString("errorMessage", tr("AI processing failed. The original recording was saved.", "AI 整理失败，原始录音已经保存。"))));
            Button retry = secondaryButton(tr("Record again", "重新录制"));
            retry.setOnClickListener(v -> deleteDraft(answer.optString("id"), question));
            card.addView(retry, fullWidth());
            return card;
        }

        TextView label = text(tr("AI SUMMARY", "AI 整理"), 12, COLOR_MUTED);
        card.addView(label);
        TextView summary = text(answer.optString("aiSummary"), 17, COLOR_TEXT);
        summary.setLineSpacing(0, 1.22f);
        summary.setPadding(0, dp(5), 0, dp(12));
        card.addView(summary);

        Button play = secondaryButton(tr("▶  Play original", "▶  播放原声"));
        play.setOnClickListener(v -> playAudio(
                answer.optString("audioUrl"),
                play,
                tr("▶  Play original", "▶  播放原声")
        ));
        card.addView(play, fullWidth());

        if ("ready".equals(answerStatus)) {
            TextView draft = text(tr("Only you can see this draft. It is shared with family only after you confirm.", "只有你能看到这份草稿。确认后才会分享给家人。"), 13, COLOR_MUTED);
            draft.setPadding(0, dp(12), 0, dp(10));
            card.addView(draft);
            LinearLayout actions = new LinearLayout(this);
            Button retry = secondaryButton(tr("Record again", "重新录制"));
            retry.setOnClickListener(v -> deleteDraft(answer.optString("id"), question));
            actions.addView(retry, new LinearLayout.LayoutParams(0, dp(52), 1));
            Button publish = primaryButton(tr("Confirm sharing", "确认分享"));
            LinearLayout.LayoutParams publishParams = new LinearLayout.LayoutParams(0, dp(52), 1);
            publishParams.setMargins(dp(10), 0, 0, 0);
            publish.setOnClickListener(v -> publishAnswer(answer.optString("id")));
            actions.addView(publish, publishParams);
            card.addView(actions, fullWidth());
        } else if ("shared".equals(answerStatus)) {
            JSONArray replies = question.optJSONArray("replies");
            if (replies != null && replies.length() > 0) {
                TextView repliesTitle = text(tr("Family replies", "家人的回复"), 13, COLOR_MUTED);
                repliesTitle.setPadding(0, dp(16), 0, dp(4));
                card.addView(repliesTitle);
                for (int i = 0; i < replies.length(); i++) {
                    JSONObject reply = replies.optJSONObject(i);
                    if (reply != null) {
                        TextView replyView = text(reply.optString("authorId") + "：" + reply.optString("text"), 15, COLOR_TEXT);
                        replyView.setPadding(0, dp(4), 0, dp(4));
                        card.addView(replyView);
                    }
                }
            }
            EditText replyInput = new EditText(this);
            replyInput.setHint(tr("Write a reply…", "写一句回复……"));
            replyInput.setTextSize(15);
            replyInput.setSingleLine(true);
            replyInput.setBackground(rounded(Color.rgb(248, 242, 237), dp(10)));
            replyInput.setPadding(dp(12), dp(8), dp(12), dp(8));
            LinearLayout.LayoutParams replyParams = fullWidth();
            replyParams.setMargins(0, dp(12), 0, dp(8));
            card.addView(replyInput, replyParams);
            Button replyButton = secondaryButton(tr("Reply to Dad", "回复爸爸"));
            replyButton.setOnClickListener(v -> createReply(answer.optString("id"), replyInput));
            card.addView(replyButton, fullWidth());
        }
        return card;
    }

    private void showLanguageSettings() {
        if (recorder != null) {
            Toast.makeText(this, tr("Finish the recording before changing language", "请先结束录音，再切换语言"), Toast.LENGTH_LONG).show();
            return;
        }
        String[] labels = {"English", "中文"};
        int selected = LanguageSettings.CHINESE.equals(language) ? 1 : 0;
        new AlertDialog.Builder(this)
                .setTitle(tr("App language", "App 语言"))
                .setSingleChoiceItems(labels, selected, (dialog, which) -> {
                    String next = which == 1 ? LanguageSettings.CHINESE : LanguageSettings.ENGLISH;
                    if (!next.equals(language)) {
                        LanguageSettings.set(this, next);
                        PhotoSync.preferences(this).edit().remove(PhotoSync.KEY_STATUS).apply();
                        dialog.dismiss();
                        recreate();
                    } else {
                        dialog.dismiss();
                    }
                })
                .setNegativeButton(tr("Cancel", "取消"), null)
                .show();
    }

    private void beginRecording(JSONObject question, Button button) {
        pendingRecordQuestion = question;
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO}, RECORD_AUDIO_REQUEST);
            return;
        }
        startRecording(question, button);
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == RECORD_AUDIO_REQUEST && grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED && pendingDailyRecordButton != null) {
            Button button = pendingDailyRecordButton;
            pendingDailyRecordButton = null;
            startDailyRecording(button);
        } else if (requestCode == RECORD_AUDIO_REQUEST && grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED && pendingRecordQuestion != null) {
            loadFeed();
            Toast.makeText(this, tr("Microphone access is on. Tap the record button again.", "权限已开启，请再次点击录音按钮"), Toast.LENGTH_LONG).show();
        } else if (requestCode == RECORD_AUDIO_REQUEST) {
            Toast.makeText(this, tr("Microphone access is required to answer", "需要麦克风权限才能回答"), Toast.LENGTH_LONG).show();
        } else if (requestCode == PHOTO_ACCESS_REQUEST) {
            if (PhotoSync.hasImageAccess(this)) {
                PhotoSync.preferences(this).edit().putBoolean(PhotoSync.KEY_ENABLED, true).apply();
                PhotoSync.schedule(this);
                PhotoSync.syncNow(this);
                refreshPhotoSyncStatus();
                pollPhotoSyncStatus(0);
            } else {
                refreshPhotoSyncStatus();
                Toast.makeText(this, tr("Photo access is required for automatic sync; you may also allow selected photos only.", "需要允许照片访问才能自动同步；你也可以只授权选中的照片"), Toast.LENGTH_LONG).show();
            }
        }
    }

    private void showPhotoSyncSettings() {
        android.content.SharedPreferences prefs = PhotoSync.preferences(this);
        LinearLayout fields = vertical();
        int padding = dp(20);
        fields.setPadding(padding, 0, padding, 0);
        EditText baseUrl = new EditText(this);
        baseUrl.setHint(tr("NAS service address, for example https://family.example.com", "NAS 服务地址，例如 https://family.example.com"));
        baseUrl.setSingleLine(true);
        baseUrl.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        baseUrl.setText(prefs.getString(PhotoSync.KEY_BASE_URL, ""));
        fields.addView(baseUrl, fullWidth());
        EditText token = new EditText(this);
        token.setHint(tr("Member token", "成员令牌"));
        token.setSingleLine(true);
        token.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        token.setText(prefs.getString(PhotoSync.KEY_MEMBER_TOKEN, ""));
        fields.addView(token, fullWidth());
        EditText lookbackDays = new EditText(this);
        lookbackDays.setHint(tr("Days of photos to sync (1–3650)", "同步最近多少天（1-3650）"));
        lookbackDays.setSingleLine(true);
        lookbackDays.setInputType(InputType.TYPE_CLASS_NUMBER);
        lookbackDays.setText(Integer.toString(PhotoSync.lookbackDays(this)));
        fields.addView(lookbackDays, fullWidth());
        AlertDialog dialog = new AlertDialog.Builder(this)
                .setTitle(tr("Connect your private Space", "连接你的私人空间"))
                .setMessage(tr("The address must be a Family Daily service reachable from this phone. The token stays in this app's private storage.", "地址必须是手机能访问的 Family Daily 服务；令牌只保存在此 App 的私有数据中。"))
                .setView(fields)
                .setNegativeButton(tr("Cancel", "取消"), null)
                .setPositiveButton(tr("Save", "保存"), null)
                .create();
        dialog.setOnShowListener(ignored -> dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(v -> {
            String url = MediaUploadClient.trimTrailingSlash(baseUrl.getText().toString());
            String memberToken = token.getText().toString().trim();
            int days;
            try {
                days = Integer.parseInt(lookbackDays.getText().toString().trim());
            } catch (NumberFormatException invalidNumber) {
                days = 0;
            }
            if (!MediaUploadClient.isValidBaseUrl(url) || memberToken.isEmpty()) {
                Toast.makeText(this, tr("Enter a complete http(s) address and member token", "请填写完整的 http(s) 地址和成员令牌"), Toast.LENGTH_LONG).show();
                return;
            }
            if (!PhotoSyncWindow.isValidDays(days)) {
                Toast.makeText(this, tr("Enter a sync range from 1 to 3650 days", "同步范围请输入 1 到 3650 天"), Toast.LENGTH_LONG).show();
                return;
            }
            String oldUrl = prefs.getString(PhotoSync.KEY_BASE_URL, "");
            String oldToken = prefs.getString(PhotoSync.KEY_MEMBER_TOKEN, "");
            int oldDays = PhotoSync.lookbackDays(this);
            android.content.SharedPreferences.Editor edit = prefs.edit()
                    .putString(PhotoSync.KEY_BASE_URL, url)
                    .putString(PhotoSync.KEY_MEMBER_TOKEN, memberToken)
                    .putInt(PhotoSync.KEY_LOOKBACK_DAYS, days)
                    .putBoolean(PhotoSync.KEY_ENABLED, true)
                    .putString(PhotoSync.KEY_STATUS, tr("Connection saved; photos from the last " + days + " days will sync", "连接已保存；将同步最近 " + days + " 天的照片"));
            if (!url.equals(oldUrl) || !memberToken.equals(oldToken) || days != oldDays
                    || !prefs.contains(PhotoSync.KEY_LOOKBACK_DAYS)) {
                edit.putLong(PhotoSync.KEY_CURSOR_SECONDS, 0).putLong(PhotoSync.KEY_CURSOR_ID, 0);
            }
            edit.apply();
            dialog.dismiss();
            refreshPhotoSyncStatus();
            enableOrSyncPhotos();
        }));
        dialog.show();
    }

    private void enableOrSyncPhotos() {
        if (!PhotoSync.isConfigured(this)) {
            showPhotoSyncSettings();
            return;
        }
        if (!PhotoSync.hasImageAccess(this)) {
            requestPermissions(PhotoSync.imagePermissions(), PHOTO_ACCESS_REQUEST);
            return;
        }
        PhotoSync.preferences(this).edit().putBoolean(PhotoSync.KEY_ENABLED, true)
                .putString(PhotoSync.KEY_STATUS, tr("Sync has been handed to the system…", "已交给系统开始同步……")).apply();
        PhotoSync.schedule(this);
        PhotoSync.syncNow(this);
        refreshPhotoSyncStatus();
        pollPhotoSyncStatus(0);
    }

    private void refreshPhotoSyncStatus() {
        if (photoSyncStatus == null || photoSyncButton == null) return;
        android.content.SharedPreferences prefs = PhotoSync.preferences(this);
        boolean configured = PhotoSync.isConfigured(this);
        boolean access = PhotoSync.hasImageAccess(this);
        String value = prefs.getString(PhotoSync.KEY_STATUS, "");
        if (!configured) value = tr("NAS service is not connected", "尚未连接 NAS 服务");
        else if (!access) value = tr("Photos from the last " + PhotoSync.lookbackDays(this) + " days will sync; allow photo access", "将同步最近 " + PhotoSync.lookbackDays(this) + " 天；请允许照片访问");
        else if (value == null || value.isEmpty()) value = tr("Enabled; photos from the last " + PhotoSync.lookbackDays(this) + " days will sync", "已开启；将同步最近 " + PhotoSync.lookbackDays(this) + " 天");
        photoSyncStatus.setText(value);
        photoSyncButton.setText(configured && access ? tr("Sync now", "立即同步") : tr("Enable sync", "开启同步"));
    }

    private void pollPhotoSyncStatus(int attempt) {
        if (attempt >= 60) return;
        handler.postDelayed(() -> {
            refreshPhotoSyncStatus();
            pollPhotoSyncStatus(attempt + 1);
        }, 1000);
    }

    private void startRecording(JSONObject question, Button button) {
        try {
            recordingFile = new File(getCacheDir(), "answer-" + UUID.randomUUID() + ".m4a");
            recorder = Build.VERSION.SDK_INT >= 31 ? new MediaRecorder(this) : new MediaRecorder();
            recorder.setAudioSource(MediaRecorder.AudioSource.MIC);
            recorder.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4);
            recorder.setAudioEncoder(MediaRecorder.AudioEncoder.AAC);
            recorder.setAudioEncodingBitRate(64_000);
            recorder.setAudioSamplingRate(44_100);
            recorder.setOutputFile(recordingFile.getAbsolutePath());
            recorder.prepare();
            recorder.start();
            button.setText(tr("■  Stop and upload", "■  结束并上传"));
            button.setBackground(rounded(Color.rgb(117, 45, 35), dp(14)));
            button.setOnClickListener(v -> stopAndUpload(question));
            Toast.makeText(this, tr("Recording — speak naturally", "正在录音，请自然地说话"), Toast.LENGTH_SHORT).show();
        } catch (Exception error) {
            releaseRecorder();
            Toast.makeText(this, tr("Unable to start recording", "暂时无法开始录音"), Toast.LENGTH_LONG).show();
        }
    }

    private void stopAndUpload(JSONObject question) {
        try {
            recorder.stop();
        } catch (RuntimeException tooShort) {
            releaseRecorder();
            if (recordingFile != null) recordingFile.delete();
            Toast.makeText(this, tr("The recording was too short. Please try again.", "录音太短，请再说一次"), Toast.LENGTH_LONG).show();
            loadFeed();
            return;
        }
        releaseRecorder();
        setBusy(true, tr("Saving and organizing the recording…", "正在保存录音并整理内容……"));
        File file = recordingFile;
        executor.execute(() -> {
            try {
                uploadAnswer(question.getString("id"), file);
                if (file != null) file.delete();
                loadFeedInBackground();
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void releaseRecorder() {
        if (recorder != null) {
            try { recorder.release(); } catch (Exception ignored) {}
            recorder = null;
        }
    }

    private void uploadAnswer(String questionId, File audio) throws Exception {
        String boundary = "FamilyDaily-" + UUID.randomUUID();
        HttpURLConnection connection = openConnection("POST", "/api/v1/questions/" + questionId + "/answer");
        connection.setRequestProperty("Content-Type", "multipart/form-data; boundary=" + boundary);
        connection.setDoOutput(true);
        try (OutputStream output = new BufferedOutputStream(connection.getOutputStream())) {
            writeUtf8(output, "--" + boundary + "\r\nContent-Disposition: form-data; name=\"answeredBy\"\r\n\r\n爸爸\r\n");
            writeUtf8(output, "--" + boundary + "\r\nContent-Disposition: form-data; name=\"audio\"; filename=\"answer.m4a\"\r\nContent-Type: audio/mp4\r\n\r\n");
            try (InputStream input = new BufferedInputStream(new FileInputStream(audio))) {
                byte[] buffer = new byte[8192];
                int read;
                while ((read = input.read(buffer)) != -1) output.write(buffer, 0, read);
            }
            writeUtf8(output, "\r\n--" + boundary + "--\r\n");
        }
        readResponse(connection);
    }

    private void publishAnswer(String answerId) {
        setBusy(true, tr("Sharing with family…", "正在分享给家人……"));
        executor.execute(() -> {
            try {
                request("POST", "/api/v1/answers/" + answerId + "/publish", new JSONObject());
                loadFeedInBackground();
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void deleteDraft(String answerId, JSONObject question) {
        new AlertDialog.Builder(this)
                .setTitle(tr("Record again?", "重新录制？"))
                .setMessage(tr("The current recording will be removed from the family draft and retained as a local historical version.", "当前录音会从家庭草稿中移除，并作为本地历史版本保留。"))
                .setNegativeButton(tr("Cancel", "取消"), null)
                .setPositiveButton(tr("Archive and record again", "归档并重录"), (dialog, which) -> {
                    setBusy(true, tr("Removing draft…", "正在删除草稿……"));
                    executor.execute(() -> {
                        try {
                            request("POST", "/api/v1/answers/" + answerId + "/archive", new JSONObject());
                            loadFeedInBackground();
                        } catch (Exception error) {
                            showError(error);
                        }
                    });
                }).show();
    }

    private void createReply(String answerId, EditText input) {
        String text = input.getText().toString().trim();
        if (text.isEmpty()) {
            input.setError(tr("Write a reply first", "先写一句回复"));
            return;
        }
        setBusy(true, tr("Sending reply…", "正在发送回复……"));
        executor.execute(() -> {
            try {
                request("POST", "/api/v1/answers/" + answerId + "/replies", new JSONObject().put("authorId", "洋宇").put("text", text));
                loadFeedInBackground();
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void playAudio(String path, Button button, String restingLabel) {
        button.setEnabled(false);
        button.setText(tr("Loading original audio…", "正在加载原声……"));
        MediaPlayer player = new MediaPlayer();
        try {
            Map<String, String> headers = new HashMap<>();
            headers.put("X-Family-Token", BuildConfig.FAMILY_API_TOKEN);
            player.setDataSource(this, android.net.Uri.parse(BuildConfig.API_BASE_URL + path), headers);
            player.setOnPreparedListener(mediaPlayer -> {
                button.setText(tr("Playing…", "正在播放……"));
                mediaPlayer.start();
            });
            player.setOnCompletionListener(mediaPlayer -> {
                mediaPlayer.release();
                button.setEnabled(true);
                button.setText(restingLabel);
            });
            player.setOnErrorListener((mediaPlayer, what, extra) -> {
                mediaPlayer.release();
                button.setEnabled(true);
                button.setText(restingLabel);
                Toast.makeText(this, tr("Unable to play the original audio", "暂时无法播放原声"), Toast.LENGTH_LONG).show();
                return true;
            });
            player.prepareAsync();
        } catch (Exception error) {
            player.release();
            button.setEnabled(true);
            button.setText(restingLabel);
            Toast.makeText(this, tr("Unable to play the original audio", "暂时无法播放原声"), Toast.LENGTH_LONG).show();
        }
    }

    private String request(String method, String path, JSONObject body) throws Exception {
        HttpURLConnection connection = openConnection(method, path);
        if (body != null) {
            connection.setDoOutput(true);
            connection.setRequestProperty("Content-Type", "application/json; charset=utf-8");
            try (OutputStream output = connection.getOutputStream()) {
                output.write(body.toString().getBytes(StandardCharsets.UTF_8));
            }
        }
        return readResponse(connection);
    }

    private HttpURLConnection openConnection(String method, String path) throws Exception {
        HttpURLConnection connection = (HttpURLConnection) new URL(BuildConfig.API_BASE_URL + path).openConnection();
        connection.setRequestMethod(method);
        connection.setConnectTimeout(10_000);
        connection.setReadTimeout(180_000);
        connection.setRequestProperty("Accept", "application/json");
        connection.setRequestProperty("Accept-Language", language);
        connection.setRequestProperty("X-Family-Token", BuildConfig.FAMILY_API_TOKEN);
        return connection;
    }

    private String readResponse(HttpURLConnection connection) throws Exception {
        int statusCode = connection.getResponseCode();
        if (statusCode == 204) return "";
        InputStream stream = statusCode >= 200 && statusCode < 300 ? connection.getInputStream() : connection.getErrorStream();
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        if (stream != null) {
            try (InputStream input = stream) {
                byte[] buffer = new byte[4096];
                int read;
                while ((read = input.read(buffer)) != -1) bytes.write(buffer, 0, read);
            }
        }
        String response = bytes.toString(StandardCharsets.UTF_8.name());
        if (statusCode < 200 || statusCode >= 300) {
            String message = tr("Request failed (" + statusCode + ")", "请求失败（" + statusCode + "）");
            try { message = new JSONObject(response).optString("error", message); } catch (Exception ignored) {}
            throw new IllegalStateException(message);
        }
        return response;
    }

    private void showError(Exception error) {
        runOnUiThread(() -> {
            setBusy(false, tr("Unable to complete: ", "暂时无法完成：") + error.getMessage());
            Toast.makeText(this, tr("Make sure the backend is running, then try again", "请确认后端正在运行，然后重试"), Toast.LENGTH_LONG).show();
        });
    }

    private void setBusy(boolean busy, String message) {
        runOnUiThread(() -> {
            progress.setVisibility(busy ? View.VISIBLE : View.GONE);
            status.setText(message);
            status.setVisibility(message.isEmpty() ? View.GONE : View.VISIBLE);
        });
    }

    private LinearLayout vertical() {
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        return layout;
    }

    private String tr(String english, String chinese) {
        return LanguageSettings.text(language, english, chinese);
    }

    private LinearLayout card() {
        LinearLayout layout = vertical();
        layout.setBackground(rounded(COLOR_CARD, dp(18)));
        layout.setElevation(dp(2));
        return layout;
    }

    private TextView text(String value, int sp, int color) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(sp);
        view.setTextColor(color);
        return view;
    }

    private TextView infoBox(String value) {
        TextView view = text(value, 14, Color.rgb(117, 45, 35));
        view.setPadding(dp(12), dp(10), dp(12), dp(10));
        view.setBackground(rounded(Color.rgb(255, 235, 229), dp(10)));
        LinearLayout.LayoutParams params = fullWidth();
        params.setMargins(0, 0, 0, dp(10));
        view.setLayoutParams(params);
        return view;
    }

    private Button primaryButton(String label) {
        Button button = new Button(this);
        button.setText(label);
        button.setTextColor(Color.WHITE);
        button.setTextSize(16);
        button.setAllCaps(false);
        button.setMinHeight(dp(52));
        button.setBackground(rounded(COLOR_PRIMARY, dp(14)));
        return button;
    }

    private Button secondaryButton(String label) {
        Button button = new Button(this);
        button.setText(label);
        button.setTextColor(COLOR_PRIMARY);
        button.setTextSize(15);
        button.setAllCaps(false);
        button.setMinHeight(dp(48));
        button.setBackground(rounded(Color.rgb(250, 239, 233), dp(12)));
        return button;
    }

    private GradientDrawable rounded(int color, int radius) {
        GradientDrawable shape = new GradientDrawable();
        shape.setColor(color);
        shape.setCornerRadius(radius);
        return shape;
    }

    private LinearLayout.LayoutParams fullWidth() {
        return new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
    }

    private LinearLayout.LayoutParams cardSpacing() {
        LinearLayout.LayoutParams params = fullWidth();
        params.setMargins(0, dp(12), 0, dp(4));
        return params;
    }

    private void writeUtf8(OutputStream output, String value) throws Exception {
        output.write(value.getBytes(StandardCharsets.UTF_8));
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
