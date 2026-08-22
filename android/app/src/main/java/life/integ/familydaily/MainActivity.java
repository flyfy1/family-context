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

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(buildScreen());
        loadFeed();
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
        TextView title = text("把一句关心，\n变成一次对话", 30, COLOR_TEXT);
        title.setPadding(0, dp(8), 0, dp(8));
        root.addView(title);
        TextView intro = text("先问爸爸一个具体的问题。他只需要点一下按钮，就能用语音回答。", 16, COLOR_MUTED);
        intro.setLineSpacing(0, 1.25f);
        root.addView(intro);

        LinearLayout syncCard = card();
        syncCard.setPadding(dp(18), dp(18), dp(18), dp(18));
        LinearLayout.LayoutParams syncCardParams = fullWidth();
        syncCardParams.setMargins(0, dp(24), 0, 0);
        root.addView(syncCard, syncCardParams);
        syncCard.addView(text("照片自动备份", 20, COLOR_TEXT));
        TextView syncIntro = text("授权后，现有和新照片会在打开 App 时补同步，并由系统定期同步到你的私人 NAS 空间。", 14, COLOR_MUTED);
        syncIntro.setLineSpacing(0, 1.2f);
        syncIntro.setPadding(0, dp(8), 0, dp(12));
        syncCard.addView(syncIntro);
        photoSyncStatus = text("尚未配置", 14, COLOR_MUTED);
        photoSyncStatus.setPadding(0, 0, 0, dp(10));
        syncCard.addView(photoSyncStatus);
        LinearLayout syncActions = new LinearLayout(this);
        Button configure = secondaryButton("连接 NAS");
        configure.setOnClickListener(v -> showPhotoSyncSettings());
        syncActions.addView(configure, new LinearLayout.LayoutParams(0, dp(52), 1));
        photoSyncButton = primaryButton("开启同步");
        LinearLayout.LayoutParams syncButtonParams = new LinearLayout.LayoutParams(0, dp(52), 1);
        syncButtonParams.setMargins(dp(10), 0, 0, 0);
        photoSyncButton.setOnClickListener(v -> enableOrSyncPhotos());
        syncActions.addView(photoSyncButton, syncButtonParams);
        syncCard.addView(syncActions, fullWidth());

        LinearLayout askCard = card();
        askCard.setPadding(dp(18), dp(18), dp(18), dp(18));
        LinearLayout.LayoutParams cardParams = fullWidth();
        cardParams.setMargins(0, dp(24), 0, dp(22));
        root.addView(askCard, cardParams);
        askCard.addView(text("问爸爸一个问题", 20, COLOR_TEXT));

        questionInput = new EditText(this);
        questionInput.setHint("例如：老张后来去医院了吗？");
        questionInput.setTextSize(16);
        questionInput.setMinHeight(dp(54));
        questionInput.setPadding(dp(14), dp(10), dp(14), dp(10));
        questionInput.setSingleLine(false);
        questionInput.setMaxLines(3);
        questionInput.setBackground(rounded(Color.rgb(248, 242, 237), dp(12)));
        LinearLayout.LayoutParams inputParams = fullWidth();
        inputParams.setMargins(0, dp(14), 0, dp(12));
        askCard.addView(questionInput, inputParams);

        Button askButton = primaryButton("发送问题");
        askButton.setOnClickListener(v -> createQuestion());
        askCard.addView(askButton, fullWidth());

        LinearLayout heading = new LinearLayout(this);
        heading.setGravity(Gravity.CENTER_VERTICAL);
        heading.addView(text("家庭动态", 22, COLOR_TEXT), new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1));
        Button refresh = secondaryButton("刷新");
        refresh.setOnClickListener(v -> loadFeed());
        heading.addView(refresh);
        root.addView(heading, fullWidth());

        progress = new ProgressBar(this);
        progress.setVisibility(View.GONE);
        LinearLayout.LayoutParams progressParams = new LinearLayout.LayoutParams(dp(28), dp(28));
        progressParams.gravity = Gravity.CENTER_HORIZONTAL;
        progressParams.setMargins(0, dp(16), 0, dp(8));
        root.addView(progress, progressParams);

        status = text("正在连接家庭空间……", 14, COLOR_MUTED);
        status.setGravity(Gravity.CENTER);
        status.setPadding(0, dp(12), 0, dp(4));
        root.addView(status, fullWidth());

        feedContainer = vertical();
        root.addView(feedContainer, fullWidth());
        return scroll;
    }

    private void createQuestion() {
        String text = questionInput.getText().toString().trim();
        if (text.isEmpty()) {
            questionInput.setError("先写下你想问的问题");
            return;
        }
        setBusy(true, "正在发送问题……");
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
                    Toast.makeText(this, "问题已经准备好", Toast.LENGTH_SHORT).show();
                });
                loadFeedInBackground();
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void loadFeed() {
        setBusy(true, "正在读取家庭动态……");
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
        setBusy(false, questions.length() == 0 ? "还没有问题。先从一句具体的关心开始吧。" : "");
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
        TextView meta = text(question.optString("askedBy") + " 想问 " + question.optString("askedTo"), 13, COLOR_PRIMARY);
        card.addView(meta);
        TextView questionText = text(question.optString("text"), 20, COLOR_TEXT);
        questionText.setLineSpacing(0, 1.18f);
        questionText.setPadding(0, dp(8), 0, dp(14));
        card.addView(questionText);

        JSONObject answer = question.optJSONObject("answer");
        if (answer == null) {
            Button record = primaryButton("🎙  回答这个问题");
            record.setMinHeight(dp(58));
            record.setTextSize(18);
            record.setOnClickListener(v -> beginRecording(question, record));
            card.addView(record, fullWidth());
            return card;
        }

        String answerStatus = answer.optString("status");
        if ("processing_failed".equals(answerStatus)) {
            card.addView(infoBox(answer.optString("errorMessage", "AI 整理失败，原始录音已经保存。")));
            Button retry = secondaryButton("重新录制");
            retry.setOnClickListener(v -> deleteDraft(answer.optString("id"), question));
            card.addView(retry, fullWidth());
            return card;
        }

        TextView label = text("AI 整理", 12, COLOR_MUTED);
        card.addView(label);
        TextView summary = text(answer.optString("aiSummary"), 17, COLOR_TEXT);
        summary.setLineSpacing(0, 1.22f);
        summary.setPadding(0, dp(5), 0, dp(12));
        card.addView(summary);

        Button play = secondaryButton("▶  播放原声");
        play.setOnClickListener(v -> playAudio(answer.optString("audioUrl"), play));
        card.addView(play, fullWidth());

        if ("ready".equals(answerStatus)) {
            TextView draft = text("只有你能看到这份草稿。确认后才会分享给家人。", 13, COLOR_MUTED);
            draft.setPadding(0, dp(12), 0, dp(10));
            card.addView(draft);
            LinearLayout actions = new LinearLayout(this);
            Button retry = secondaryButton("重新录制");
            retry.setOnClickListener(v -> deleteDraft(answer.optString("id"), question));
            actions.addView(retry, new LinearLayout.LayoutParams(0, dp(52), 1));
            Button publish = primaryButton("确认分享");
            LinearLayout.LayoutParams publishParams = new LinearLayout.LayoutParams(0, dp(52), 1);
            publishParams.setMargins(dp(10), 0, 0, 0);
            publish.setOnClickListener(v -> publishAnswer(answer.optString("id")));
            actions.addView(publish, publishParams);
            card.addView(actions, fullWidth());
        } else if ("shared".equals(answerStatus)) {
            JSONArray replies = question.optJSONArray("replies");
            if (replies != null && replies.length() > 0) {
                TextView repliesTitle = text("家人的回复", 13, COLOR_MUTED);
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
            replyInput.setHint("写一句回复……");
            replyInput.setTextSize(15);
            replyInput.setSingleLine(true);
            replyInput.setBackground(rounded(Color.rgb(248, 242, 237), dp(10)));
            replyInput.setPadding(dp(12), dp(8), dp(12), dp(8));
            LinearLayout.LayoutParams replyParams = fullWidth();
            replyParams.setMargins(0, dp(12), 0, dp(8));
            card.addView(replyInput, replyParams);
            Button replyButton = secondaryButton("回复爸爸");
            replyButton.setOnClickListener(v -> createReply(answer.optString("id"), replyInput));
            card.addView(replyButton, fullWidth());
        }
        return card;
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
        if (requestCode == RECORD_AUDIO_REQUEST && grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED && pendingRecordQuestion != null) {
            loadFeed();
            Toast.makeText(this, "权限已开启，请再次点击录音按钮", Toast.LENGTH_LONG).show();
        } else if (requestCode == RECORD_AUDIO_REQUEST) {
            Toast.makeText(this, "需要麦克风权限才能回答", Toast.LENGTH_LONG).show();
        } else if (requestCode == PHOTO_ACCESS_REQUEST) {
            if (PhotoSync.hasImageAccess(this)) {
                PhotoSync.preferences(this).edit().putBoolean(PhotoSync.KEY_ENABLED, true).apply();
                PhotoSync.schedule(this);
                PhotoSync.syncNow(this);
                refreshPhotoSyncStatus();
                pollPhotoSyncStatus(0);
            } else {
                refreshPhotoSyncStatus();
                Toast.makeText(this, "需要允许照片访问才能自动同步；你也可以只授权选中的照片", Toast.LENGTH_LONG).show();
            }
        }
    }

    private void showPhotoSyncSettings() {
        android.content.SharedPreferences prefs = PhotoSync.preferences(this);
        LinearLayout fields = vertical();
        int padding = dp(20);
        fields.setPadding(padding, 0, padding, 0);
        EditText baseUrl = new EditText(this);
        baseUrl.setHint("NAS 服务地址，例如 https://family.example.com");
        baseUrl.setSingleLine(true);
        baseUrl.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        baseUrl.setText(prefs.getString(PhotoSync.KEY_BASE_URL, ""));
        fields.addView(baseUrl, fullWidth());
        EditText token = new EditText(this);
        token.setHint("成员令牌");
        token.setSingleLine(true);
        token.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        token.setText(prefs.getString(PhotoSync.KEY_MEMBER_TOKEN, ""));
        fields.addView(token, fullWidth());
        EditText lookbackDays = new EditText(this);
        lookbackDays.setHint("同步最近多少天（1-3650）");
        lookbackDays.setSingleLine(true);
        lookbackDays.setInputType(InputType.TYPE_CLASS_NUMBER);
        lookbackDays.setText(Integer.toString(PhotoSync.lookbackDays(this)));
        fields.addView(lookbackDays, fullWidth());
        AlertDialog dialog = new AlertDialog.Builder(this)
                .setTitle("连接你的私人空间")
                .setMessage("地址必须是手机能访问的 Family Daily 服务；令牌只保存在此 App 的私有数据中。")
                .setView(fields)
                .setNegativeButton("取消", null)
                .setPositiveButton("保存", null)
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
                Toast.makeText(this, "请填写完整的 http(s) 地址和成员令牌", Toast.LENGTH_LONG).show();
                return;
            }
            if (!PhotoSyncWindow.isValidDays(days)) {
                Toast.makeText(this, "同步范围请输入 1 到 3650 天", Toast.LENGTH_LONG).show();
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
                    .putString(PhotoSync.KEY_STATUS, "连接已保存；将同步最近 " + days + " 天的照片");
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
                .putString(PhotoSync.KEY_STATUS, "已交给系统开始同步……").apply();
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
        if (!configured) value = "尚未连接 NAS 服务";
        else if (!access) value = "将同步最近 " + PhotoSync.lookbackDays(this) + " 天；请允许照片访问";
        else if (value == null || value.isEmpty()) value = "已开启；将同步最近 " + PhotoSync.lookbackDays(this) + " 天";
        photoSyncStatus.setText(value);
        photoSyncButton.setText(configured && access ? "立即同步" : "开启同步");
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
            button.setText("■  结束并上传");
            button.setBackground(rounded(Color.rgb(117, 45, 35), dp(14)));
            button.setOnClickListener(v -> stopAndUpload(question));
            Toast.makeText(this, "正在录音，请自然地说话", Toast.LENGTH_SHORT).show();
        } catch (Exception error) {
            releaseRecorder();
            Toast.makeText(this, "暂时无法开始录音", Toast.LENGTH_LONG).show();
        }
    }

    private void stopAndUpload(JSONObject question) {
        try {
            recorder.stop();
        } catch (RuntimeException tooShort) {
            releaseRecorder();
            if (recordingFile != null) recordingFile.delete();
            Toast.makeText(this, "录音太短，请再说一次", Toast.LENGTH_LONG).show();
            loadFeed();
            return;
        }
        releaseRecorder();
        setBusy(true, "正在保存录音并整理内容……");
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
        setBusy(true, "正在分享给家人……");
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
                .setTitle("重新录制？")
                .setMessage("当前录音会从家庭草稿中移除，并作为本地历史版本保留。")
                .setNegativeButton("取消", null)
                .setPositiveButton("归档并重录", (dialog, which) -> {
                    setBusy(true, "正在删除草稿……");
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
            input.setError("先写一句回复");
            return;
        }
        setBusy(true, "正在发送回复……");
        executor.execute(() -> {
            try {
                request("POST", "/api/v1/answers/" + answerId + "/replies", new JSONObject().put("authorId", "洋宇").put("text", text));
                loadFeedInBackground();
            } catch (Exception error) {
                showError(error);
            }
        });
    }

    private void playAudio(String path, Button button) {
        button.setEnabled(false);
        button.setText("正在加载原声……");
        MediaPlayer player = new MediaPlayer();
        try {
            Map<String, String> headers = new HashMap<>();
            headers.put("X-Family-Token", BuildConfig.FAMILY_API_TOKEN);
            player.setDataSource(this, android.net.Uri.parse(BuildConfig.API_BASE_URL + path), headers);
            player.setOnPreparedListener(mediaPlayer -> {
                button.setText("正在播放……");
                mediaPlayer.start();
            });
            player.setOnCompletionListener(mediaPlayer -> {
                mediaPlayer.release();
                button.setEnabled(true);
                button.setText("▶  播放原声");
            });
            player.setOnErrorListener((mediaPlayer, what, extra) -> {
                mediaPlayer.release();
                button.setEnabled(true);
                button.setText("▶  播放原声");
                Toast.makeText(this, "暂时无法播放原声", Toast.LENGTH_LONG).show();
                return true;
            });
            player.prepareAsync();
        } catch (Exception error) {
            player.release();
            button.setEnabled(true);
            button.setText("▶  播放原声");
            Toast.makeText(this, "暂时无法播放原声", Toast.LENGTH_LONG).show();
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
        connection.setReadTimeout(90_000);
        connection.setRequestProperty("Accept", "application/json");
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
            String message = "请求失败（" + statusCode + "）";
            try { message = new JSONObject(response).optString("error", message); } catch (Exception ignored) {}
            throw new IllegalStateException(message);
        }
        return response;
    }

    private void showError(Exception error) {
        runOnUiThread(() -> {
            setBusy(false, "暂时无法完成：" + error.getMessage());
            Toast.makeText(this, "请确认后端正在运行，然后重试", Toast.LENGTH_LONG).show();
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
