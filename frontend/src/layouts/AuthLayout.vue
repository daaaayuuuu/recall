<template>
  <main class="centered-page auth-page">
    <section class="panel panel--narrow auth-panel">
      <RouterLink
        class="brand brand--recall"
        to="/"
        aria-label="留刻首页"
      >
        <span
          class="prototype-brand-mark"
          aria-hidden="true"
        ><i /><em /></span>
        <span class="prototype-brand-copy">
          <strong>留刻</strong>
          <small>RECALL</small>
        </span>
      </RouterLink>
      <div
        class="auth-keepsake"
        aria-hidden="true"
      >
        <figure class="prototype-auth-photo prototype-photo-a">
          <img
            src="/assets/login-first-meeting.png"
            alt=""
          >
        </figure>
        <figure class="prototype-auth-photo prototype-photo-b">
          <img
            src="/assets/login-eat-together.png"
            alt=""
          >
        </figure>
        <span class="prototype-paper-heart">♡</span>
      </div>
      <div class="auth-panel__content">
        <RouterView />
      </div>
    </section>
  </main>
</template>
