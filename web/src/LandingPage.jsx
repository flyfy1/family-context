import "./landing.css";
import heroImage from "./assets/family-memory-hero.jpg";

const copy = {
  en: {
    nav: ["How it works", "Capabilities", "Privacy"],
    navTargets: ["#how-it-works", "#capabilities", "#privacy"],
    enter: "Enter family space",
    eyebrow: "A calmer place for family stories",
    title: <>Turn everyday voices into <em>family memories.</em></>,
    intro: "One thoughtful question helps a parent start talking. Family Daily keeps the original voice, organizes it with AI, and gives everyone an easy way to respond.",
    primary: "Open Family Daily",
    secondary: "See how it works",
    privacyNote: "Private by default · Stored on your family server",
    imageAlt: "A warm kitchen table with a phone recording a voice message beside family photos and a notebook",
    imageLabel: "A voice worth keeping",
    quote: "We played chess again yesterday. He still remembers every opening.",
    quoteMeta: "Dad's original voice · AI-organized note",
    proof: [
      ["Voice first", "No long typing or complicated forms"],
      ["AI organized", "A clear summary, never a replacement for the original"],
      ["Family only", "You decide what stays private and what gets shared"],
    ],
    howEyebrow: "ONE SMALL QUESTION, A REAL CONVERSATION",
    howTitle: "Keeping in touch should feel this natural.",
    howIntro: "Family Daily removes the blank-page problem. A simple prompt becomes a story your family can hear, read, and continue.",
    steps: [
      ["01", "Ask", "Send one specific question that is easy to answer."],
      ["02", "Speak", "A parent taps the large button and answers naturally."],
      ["03", "Remember", "AI transcribes and gently organizes the answer for confirmation."],
      ["04", "Reconnect", "Family hears the original voice, replies, and keeps the topic alive."],
    ],
    capabilityEyebrow: "BUILT FOR REAL FAMILY LIFE",
    capabilityTitle: "The warmth of a call, without needing everyone free at once.",
    capabilities: [
      ["声", "Effortless voice answers", "A large, clear recorder helps older family members share without learning a new workflow."],
      ["✦", "AI that stays in its place", "Transcripts and concise summaries make stories easier to read, while the original voice remains the source."],
      ["⌂", "A living family timeline", "Photos, voice notes, daily moments, and replies stay together instead of disappearing in a busy group chat."],
    ],
    privacyEyebrow: "YOUR FAMILY, YOUR DATA",
    privacyTitle: "Made for memories that should not become content.",
    privacyBody: "Family Daily keeps authoritative data—recordings, transcripts, summaries, and history—on your own family server. Each member has a separate space, and sharing is always a choice.",
    privacyPoints: ["Local-first family storage", "Original recordings preserved", "Private and family visibility controls"],
    ctaEyebrow: "A QUESTION IS ENOUGH TO BEGIN",
    ctaTitle: "What story would you like to hear today?",
    ctaBody: "Open your family space, ask something small, and make room for an answer worth keeping.",
    ctaButton: "Go to family feed",
    footer: "Speak · Share · Remember · Reconnect",
  },
  zh: {
    nav: ["如何使用", "核心能力", "隐私保护"],
    navTargets: ["#how-it-works", "#capabilities", "#privacy"],
    enter: "进入家庭空间",
    eyebrow: "一个安静保存家庭故事的地方",
    title: <>让日常的声音，变成<em>一家人的记忆。</em></>,
    intro: "从一个具体的问题开始，让父母自然地说起来。Family Daily 保存原声、用 AI 帮忙整理，也让每位家人都能轻松回应。",
    primary: "打开 Family Daily",
    secondary: "看看如何使用",
    privacyNote: "默认私密 · 保存在你的家庭服务器",
    imageAlt: "温暖的厨房餐桌上，手机正在录制语音，旁边放着家庭照片和笔记本",
    imageLabel: "一个值得留下的声音",
    quote: "我们昨天又一起下棋了，他还是记得每一种开局。",
    quoteMeta: "爸爸的原声 · AI 整理",
    proof: [
      ["先说话", "不用长篇打字，也没有复杂表单"],
      ["AI 整理", "内容更清楚，但永远不替代原声"],
      ["只给家人", "由你决定什么私密、什么分享"],
    ],
    howEyebrow: "一个小问题，一次真正的交流",
    howTitle: "和家人保持联系，本来就该这么自然。",
    howIntro: "Family Daily 解决“想关心，却不知道说什么”的难题。一个简单问题，会变成家人可以听见、读懂并继续聊下去的故事。",
    steps: [
      ["01", "问一问", "发出一个具体、容易回答的小问题。"],
      ["02", "说一说", "父母按下大按钮，像平时聊天一样回答。"],
      ["03", "记下来", "AI 转写并谨慎整理，确认后再分享。"],
      ["04", "聊下去", "家人听原声、读摘要、回复并继续提问。"],
    ],
    capabilityEyebrow: "为真实的家庭生活而做",
    capabilityTitle: "保留电话里的温度，又不必等所有人同时有空。",
    capabilities: [
      ["声", "轻松的语音回答", "醒目、清楚的大录音按钮，让长辈不用学习复杂操作也能分享。"],
      ["✦", "恰到好处的 AI", "转写和简短摘要让故事更容易读懂，原始声音始终是内容的来源。"],
      ["⌂", "持续生长的家庭时间线", "照片、语音、日常片段和回应放在一起，不再淹没在忙碌的群聊里。"],
    ],
    privacyEyebrow: "你的家庭，你的数据",
    privacyTitle: "家庭记忆，不应该变成平台内容。",
    privacyBody: "Family Daily 把录音、转写、摘要和历史记录保存在你自己的家庭服务器。每位成员都有独立空间，每一次分享都由自己决定。",
    privacyPoints: ["本地优先的家庭存储", "原始录音完整保留", "私密与家庭可见自由选择"],
    ctaEyebrow: "从一个问题开始就够了",
    ctaTitle: "今天，你最想听家人讲什么？",
    ctaBody: "进入家庭空间，问一件小事，给一个值得留下的回答腾出位置。",
    ctaButton: "进入家庭动态",
    footer: "说出来 · 分享它 · 记下来 · 再靠近一点",
  },
};

export default function LandingPage({ language, onLanguageChange }) {
  const c = copy[language] || copy.en;
  return <div className="landing-page">
    <header className="landing-nav">
      <a className="landing-brand" href="#/" aria-label="Family Daily home"><span>家</span><strong>Family Daily</strong></a>
      <nav aria-label={language === "zh" ? "首页导航" : "Landing navigation"}>
        {c.nav.map((label, index) => <a key={label} href={c.navTargets[index]}>{label}</a>)}
      </nav>
      <div className="landing-nav-actions">
        <label className="landing-language"><span className="sr-only">Language</span><select value={language} onChange={event => onLanguageChange(event.target.value)} aria-label="Language"><option value="en">EN</option><option value="zh">中文</option></select></label>
        <a className="landing-enter" href="#/feed">{c.enter} <span aria-hidden="true">↗</span></a>
      </div>
    </header>

    <main>
      <section className="landing-hero">
        <div className="landing-hero-copy">
          <p className="landing-kicker"><span>✦</span>{c.eyebrow}</p>
          <h1>{c.title}</h1>
          <p className="landing-intro">{c.intro}</p>
          <div className="landing-hero-actions"><a className="landing-button primary" href="#/feed">{c.primary}<span>→</span></a><a className="landing-button ghost" href="#how-it-works">{c.secondary}<span>↓</span></a></div>
          <p className="landing-private-note"><span>⌂</span>{c.privacyNote}</p>
        </div>
        <figure className="landing-visual">
          <img src={heroImage} alt={c.imageAlt} />
          <figcaption><span className="landing-wave" aria-hidden="true"><i /><i /><i /><i /><i /></span><div><small>{c.imageLabel}</small><strong>{c.quote}</strong><span>{c.quoteMeta}</span></div></figcaption>
        </figure>
      </section>

      <section className="landing-proof" aria-label={language === "zh" ? "核心价值" : "Core values"}>{c.proof.map(([title, body], index) => <article key={title}><span>0{index + 1}</span><div><strong>{title}</strong><p>{body}</p></div></article>)}</section>

      <section className="landing-section landing-how" id="how-it-works">
        <div className="landing-section-heading"><div><p>{c.howEyebrow}</p><h2>{c.howTitle}</h2></div><p>{c.howIntro}</p></div>
        <div className="landing-steps">{c.steps.map(([number, title, body], index) => <article key={title}><div className="landing-step-top"><span>{number}</span><i aria-hidden="true">{index === 0 ? "?" : index === 1 ? "◉" : index === 2 ? "✦" : "♡"}</i></div><h3>{title}</h3><p>{body}</p>{index < c.steps.length - 1 && <b aria-hidden="true">→</b>}</article>)}</div>
      </section>

      <section className="landing-section landing-capabilities" id="capabilities">
        <div className="landing-section-heading compact"><div><p>{c.capabilityEyebrow}</p><h2>{c.capabilityTitle}</h2></div></div>
        <div className="landing-feature-grid">{c.capabilities.map(([icon, title, body], index) => <article key={title} className={index === 1 ? "featured" : ""}><span>{icon}</span><h3>{title}</h3><p>{body}</p></article>)}</div>
      </section>

      <section className="landing-privacy" id="privacy">
        <div className="landing-privacy-mark" aria-hidden="true"><span>⌂</span><i>♡</i></div>
        <div><p className="landing-section-label">{c.privacyEyebrow}</p><h2>{c.privacyTitle}</h2><p className="landing-privacy-body">{c.privacyBody}</p><ul>{c.privacyPoints.map(item => <li key={item}><span>✓</span>{item}</li>)}</ul></div>
      </section>

      <section className="landing-cta">
        <p>{c.ctaEyebrow}</p><h2>{c.ctaTitle}</h2><span>{c.ctaBody}</span><a className="landing-button light" href="#/feed">{c.ctaButton}<b>→</b></a>
      </section>
    </main>

    <footer className="landing-footer"><a className="landing-brand" href="#/"><span>家</span><strong>Family Daily</strong></a><p>{c.footer}</p><small>© {new Date().getFullYear()} Family Daily</small></footer>
  </div>;
}
