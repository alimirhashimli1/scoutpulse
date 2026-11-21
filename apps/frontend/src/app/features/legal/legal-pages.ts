import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { RouterLink } from '@angular/router';

import { Seo } from '../../core/seo/seo';
import { LEGAL } from './legal-config';

/**
 * The about, contact and legal pages.
 *
 * Plain text, server rendered, indexable. The facts in the privacy and cookie
 * pages describe what this codebase actually does -- which accounts it keeps,
 * what it stores in the browser, which services it runs on -- so a change to
 * any of those (adding analytics, say, or a new sign-in provider) has to be
 * reflected here too.
 *
 * Operator details come from legal-config.ts, which must be filled in.
 */

const PROSE = `
  .legal {
    max-width: 46rem;
    padding-block: var(--space-6) var(--space-8);
  }
  .eyebrow {
    font-size: var(--text-xs);
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--accent);
    margin-bottom: var(--space-2);
  }
  h1 {
    font-size: var(--text-3xl);
    margin-bottom: var(--space-2);
  }
  .updated {
    color: var(--muted);
    font-size: var(--text-sm);
    margin-bottom: var(--space-6);
  }
  h2 {
    font-size: var(--text-xl);
    margin: var(--space-6) 0 var(--space-3);
  }
  h3 {
    font-size: var(--text-lg);
    margin: var(--space-4) 0 var(--space-2);
  }
  p,
  li {
    line-height: 1.65;
    color: var(--ink-soft);
  }
  p + p {
    margin-top: var(--space-3);
  }
  ul {
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .table-wrap {
    overflow-x: auto;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }
  th,
  td {
    text-align: left;
    vertical-align: top;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--line);
  }
  th {
    color: var(--muted);
    font-weight: 600;
  }
  code {
    font-family: var(--font-mono);
    font-size: 0.92em;
    overflow-wrap: anywhere;
  }
  address {
    font-style: normal;
    line-height: 1.6;
  }
`;

@Component({
  selector: 'app-about-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <main class="page legal">
      <p class="eyebrow">About</p>
      <h1>About ScoutPulse</h1>
      <p class="updated">A free football database, analysis desk and games, in one place.</p>

      <p>
        ScoutPulse is an independent football site. It keeps a searchable record of players, clubs,
        coaches and competitions, follows every recorded transfer and market valuation, publishes
        tactical analysis, gathers the day's football news, and hosts football games you can play
        alone or with friends.
      </p>

      <h2>What you can do here</h2>
      <ul>
        <li><a routerLink="/">Follow transfers</a> — fees, loans and free moves, newest first.</li>
        <li>
          Look up <a routerLink="/clubs">clubs</a>, <a routerLink="/competitions">competitions</a>
          and players, with their career history and value over time.
        </li>
        <li><a routerLink="/articles">Read analysis</a> from the ScoutPulse desk.</li>
        <li><a routerLink="/news">Catch up on football news</a> from trusted sources.</li>
        <li>Draw and share tactics on a 3D board.</li>
        <li><a routerLink="/games">Play football games</a>, including online with friends.</li>
      </ul>

      <h2>Where the data comes from</h2>
      <p>
        Records are compiled from publicly available football data and maintained by ScoutPulse
        editors. ScoutPulse is in beta: some records are incomplete, and figures such as market
        values are estimates, not official numbers. If you spot a mistake, please
        <a routerLink="/contact">tell us</a>.
      </p>
      <p>
        ScoutPulse is not affiliated with, endorsed by or sponsored by any club, league, federation
        or player. Club and competition names, crests and trademarks belong to their owners and are
        used only to identify them. News headlines link to the publisher that wrote them.
      </p>

      <h2>Who runs it</h2>
      <address>
        {{ legal.operator }}<br />
        @for (line of legal.address; track $index) {
          {{ line }}<br />
        }
        Email: <a [href]="'mailto:' + legal.email">{{ legal.email }}</a>
      </address>
      <p>
        See also the <a routerLink="/privacy">privacy policy</a>,
        <a routerLink="/terms">terms of use</a> and <a routerLink="/cookies">cookie policy</a>.
      </p>
    </main>
  `,
  styles: PROSE,
})
export class AboutPage {
  protected readonly legal = LEGAL;
  constructor() {
    inject(Seo).describe({
      title: 'About',
      description:
        'What ScoutPulse is: a free football database of players, clubs and transfers, with tactical analysis, news and football games.',
      path: '/about',
    });
  }
}

@Component({
  selector: 'app-contact-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <main class="page legal">
      <p class="eyebrow">Contact</p>
      <h1>Contact and legal notice</h1>
      <p class="updated">Last updated {{ legal.updated }}</p>

      <h2>Get in touch</h2>
      <p>
        For anything — a wrong figure, a missing player, a takedown request, a question about your
        data — email <a [href]="'mailto:' + legal.email">{{ legal.email }}</a
        >. We aim to reply within a few working days, and to privacy requests within one month, as
        the GDPR requires.
      </p>

      <h3>Privacy requests</h3>
      <p>
        You can delete your account yourself from your <a routerLink="/account">account page</a>.
        For a copy of your data or any other request under the GDPR, email us from the address on
        your account so we can confirm it is you.
      </p>

      <h3>Reporting content</h3>
      <p>
        To report content that is unlawful or infringes your rights, email us with the page's
        address and what is wrong with it. We review every report and remove content that breaks the
        law or our <a routerLink="/terms">terms</a>.
      </p>

      <h2>Legal notice</h2>
      <p>Responsible for this website and its content:</p>
      <address>
        {{ legal.operator }}<br />
        @for (line of legal.address; track $index) {
          {{ line }}<br />
        }
        Email: <a [href]="'mailto:' + legal.email">{{ legal.email }}</a>
      </address>
      <p>
        EU online dispute resolution: we are neither obliged nor willing to take part in dispute
        resolution proceedings before a consumer arbitration board.
      </p>
    </main>
  `,
  styles: PROSE,
})
export class ContactPage {
  protected readonly legal = LEGAL;
  constructor() {
    inject(Seo).describe({
      title: 'Contact',
      description:
        'How to reach ScoutPulse: corrections, privacy requests, content reports and legal notice.',
      path: '/contact',
    });
  }
}

@Component({
  selector: 'app-privacy-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <main class="page legal">
      <p class="eyebrow">Legal</p>
      <h1>Privacy policy</h1>
      <p class="updated">Last updated {{ legal.updated }}</p>

      <p>
        This policy explains what personal data ScoutPulse processes, why, and the rights you have
        under the EU General Data Protection Regulation (GDPR) and similar laws. In short: we
        collect only what the site needs in order to work, we show no ads, we use no analytics or
        tracking, and we never sell your data.
      </p>

      <h2>1. Who is responsible</h2>
      <p>The controller of your personal data is:</p>
      <address>
        {{ legal.operator }}<br />
        @for (line of legal.address; track $index) {
          {{ line }}<br />
        }
        Email: <a [href]="'mailto:' + legal.email">{{ legal.email }}</a>
      </address>

      <h2>2. What we process and why</h2>

      <h3>Visiting the site</h3>
      <p>
        Every request reaches our servers with your IP address, the page requested, the time and
        your browser's user agent. We use this to deliver the page, keep the service secure, enforce
        rate limits against abuse and find faults. Server logs are kept for a short time by our
        hosting providers and then deleted automatically. Legal basis: our legitimate interest in
        running a secure, working website (Art. 6(1)(f) GDPR).
      </p>

      <h3>Your account</h3>
      <p>
        If you register, we store your username, email address, a one-way hash of your password
        (never the password itself), your role, and when the account was created. If you sign in
        with Google or Facebook, we receive your account identifier and email address from that
        provider, and nothing else. We send email only to confirm your address and for messages
        about your account. Legal basis: providing the service you signed up for (Art. 6(1)(b)
        GDPR).
      </p>

      <h3>What you create</h3>
      <p>
        Notes, tactics boards and articles you write are stored with your account and the name you
        wrote them under. Published articles are public and show their author's name.
      </p>

      <h3>Online games</h3>
      <p>
        When you join an online game, the nickname you type is shared with the other players in that
        room. Rooms are held in memory only and disappear when the game ends; nothing about them is
        written to a database.
      </p>

      <h3>Protection against bots</h3>
      <p>
        Sign-up and sign-in may show a challenge from Cloudflare Turnstile to tell people from
        automated scripts. Cloudflare processes technical data about your browser and connection for
        this purpose only. Legal basis: our legitimate interest in protecting accounts from abuse
        (Art. 6(1)(f) GDPR). See
        <a
          href="https://www.cloudflare.com/turnstile-privacy-policy/"
          rel="noopener"
          target="_blank"
          >Cloudflare's Turnstile privacy notice</a
        >.
      </p>

      <h3>Content from other sites</h3>
      <p>
        Some images, such as news pictures, article covers and club crests, load directly from the
        site that hosts them, which therefore sees your IP address. Videos in articles show only a
        still picture until you click play. Only then does the player load from YouTube
        (privacy-enhanced mode) or Vimeo, under their own privacy policies. Legal basis: your
        consent, given by clicking play (Art. 6(1)(a) GDPR).
      </p>

      <h2>3. Who processes data for us</h2>
      <p>We use these providers, each bound by a data processing agreement:</p>
      <ul>
        <li><strong>Vercel Inc.</strong>: hosts and delivers the website.</li>
        <li><strong>Railway Corp.</strong>: runs our servers and databases.</li>
        <li>
          <strong>Cloudflare, Inc.</strong>: bot protection, and AI summaries of public news
          articles (no personal data involved).
        </li>
        <li><strong>Our email provider</strong>: delivers account emails.</li>
        <li><strong>Google / Meta</strong>: only if you choose to sign in with them.</li>
      </ul>
      <p>
        Some of these companies are based in the United States. Transfers rely on the EU–US Data
        Privacy Framework or on the European Commission's Standard Contractual Clauses (Art. 45–46
        GDPR).
      </p>

      <h2>4. How long we keep it</h2>
      <ul>
        <li>Account data: until you delete your account.</li>
        <li>
          Sessions: a sign-in session lasts up to 30 days, or until you sign out, and is deleted
          with your account.
        </li>
        <li>Server logs: a short period set by our hosting providers, then deleted.</li>
        <li>
          After you delete your account: your notes, tactics boards and unpublished drafts are
          deleted too. Published articles stay online under the name they were published with. Email
          us if you want one taken down.
        </li>
      </ul>

      <h2>5. Your rights</h2>
      <p>You have the right to:</p>
      <ul>
        <li>access the personal data we hold about you (Art. 15);</li>
        <li>have it corrected (Art. 16);</li>
        <li>
          have it erased (Art. 17). You can delete your account at any time from your
          <a routerLink="/account">account page</a>;
        </li>
        <li>restrict processing (Art. 18);</li>
        <li>receive your data in a portable format (Art. 20);</li>
        <li>object to processing based on legitimate interests (Art. 21);</li>
        <li>withdraw consent at any time, without affecting what was done before;</li>
        <li>
          complain to a data protection supervisory authority, in particular in the EU country where
          you live or work.
        </li>
      </ul>
      <p>
        To use any of these rights, email
        <a [href]="'mailto:' + legal.email">{{ legal.email }}</a
        >. We answer within one month.
      </p>

      <h2>6. Cookies and local storage</h2>
      <p>
        We use only what the site needs in order to work. There are no advertising or analytics
        cookies. The full list is in our <a routerLink="/cookies">cookie policy</a>.
      </p>

      <h2>7. Children</h2>
      <p>You must be at least 16 to create an account. Anyone can browse and play without one.</p>

      <h2>8. Changes</h2>
      <p>
        If this policy changes, the date at the top changes with it. We will tell account holders by
        email about any significant change.
      </p>
    </main>
  `,
  styles: PROSE,
})
export class PrivacyPage {
  protected readonly legal = LEGAL;
  constructor() {
    inject(Seo).describe({
      title: 'Privacy policy',
      description:
        'What personal data ScoutPulse processes, why, for how long, and your rights under the GDPR.',
      path: '/privacy',
    });
  }
}

@Component({
  selector: 'app-terms-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <main class="page legal">
      <p class="eyebrow">Legal</p>
      <h1>Terms of use</h1>
      <p class="updated">Last updated {{ legal.updated }}</p>

      <p>
        These terms apply to everyone who uses ScoutPulse. By using the site you agree to them. If
        you do not agree, please do not use the site.
      </p>

      <h2>1. The service</h2>
      <p>
        ScoutPulse is a free football information site, offered as it is. It is in beta. We work to
        keep it accurate and available, but we do not guarantee that any figure is correct or
        complete, or that the site will always be online. Market values are estimates. Do not rely
        on them for betting, financial or contractual decisions.
      </p>

      <h2>2. Accounts</h2>
      <ul>
        <li>You must be at least 16 to register.</li>
        <li>
          Keep your sign-in details to yourself. You are responsible for what happens under your
          account.
        </li>
        <li>One person, one account. Accounts created automatically or in bulk will be removed.</li>
        <li>
          You can delete your account at any time from your
          <a routerLink="/account">account page</a>. We may suspend or remove accounts that break
          these terms.
        </li>
      </ul>

      <h2>3. Your content</h2>
      <p>
        You keep ownership of the notes, boards and articles you create. By publishing something on
        ScoutPulse, you give us a non-exclusive, royalty-free licence to host, display and
        distribute it on the site for as long as it is published. You confirm that you have the
        right to publish it.
      </p>

      <h2>4. Acceptable use</h2>
      <p>Do not:</p>
      <ul>
        <li>
          post anything unlawful, hateful, harassing, defamatory, or that infringes someone's
          rights;
        </li>
        <li>impersonate others or mislead people about who you are;</li>
        <li>
          scrape the site in bulk, overload it, get around its rate limits or bot protection, or
          probe it for vulnerabilities without permission;
        </li>
        <li>use the site to send spam or malware.</li>
      </ul>

      <h2>5. Third-party content and trademarks</h2>
      <p>
        News headlines link to the publishers that wrote them, and their content belongs to those
        publishers. Club and competition names, crests and trademarks belong to their owners.
        ScoutPulse is not affiliated with them. To report content that infringes your rights, see
        the <a routerLink="/contact">contact page</a>.
      </p>

      <h2>6. Liability</h2>
      <p>
        We are fully liable for intent and gross negligence, and for injury to life, body or health.
        In all other cases, and as far as the law allows, ScoutPulse is not liable for indirect
        loss, lost profit or lost data arising from use of the site, nor for the content of external
        sites we link to. Nothing in these terms limits rights that consumer law gives you and that
        cannot be waived.
      </p>

      <h2>7. Changes and law</h2>
      <p>
        We may update these terms, and the date at the top will change when we do. Continuing to use
        the site after a change means you accept the new terms. These terms are governed by the laws
        of {{ legal.jurisdiction }}. If you are a consumer, you also keep the protection of the
        mandatory laws of the country where you live.
      </p>
    </main>
  `,
  styles: PROSE,
})
export class TermsPage {
  protected readonly legal = LEGAL;
  constructor() {
    inject(Seo).describe({
      title: 'Terms of use',
      description:
        'The rules for using ScoutPulse: accounts, your content, acceptable use and liability.',
      path: '/terms',
    });
  }
}

@Component({
  selector: 'app-cookies-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <main class="page legal">
      <p class="eyebrow">Legal</p>
      <h1>Cookie policy</h1>
      <p class="updated">Last updated {{ legal.updated }}</p>

      <p>
        ScoutPulse uses no advertising, analytics or tracking cookies. Everything below is strictly
        necessary for a feature you use, or remembers a choice you made. Under the EU ePrivacy rules
        that does not need your consent, which is why there is no cookie banner.
      </p>

      <h2>Cookies</h2>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Purpose</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><code>sp_refresh</code> (HttpOnly)</td>
              <td>Keeps you signed in. Set only when you sign in.</td>
              <td>30 days, or until you sign out</td>
            </tr>
            <tr>
              <td><code>sp_oauth_state</code>, <code>sp_oauth_verifier</code></td>
              <td>Protect a Google or Facebook sign-in against forgery while it is in progress.</td>
              <td>10 minutes</td>
            </tr>
            <tr>
              <td>Cloudflare Turnstile</td>
              <td>
                Bot protection on the sign-up and sign-in forms. Cloudflare states it uses no
                tracking cookies.
              </td>
              <td>Session</td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2>Local storage in your browser</h2>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Key</th>
              <th>Purpose</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><code>scoutpulse.theme</code></td>
              <td>Your light or dark theme choice.</td>
            </tr>
            <tr>
              <td><code>scoutpulse.signed_in</code></td>
              <td>Whether a session may exist, so the site knows whether to restore it.</td>
            </tr>
            <tr>
              <td><code>scoutpulse.games.*</code></td>
              <td>Your game settings, nickname and current online seat.</td>
            </tr>
            <tr>
              <td>Tactics board tutorial</td>
              <td>Whether you have already seen the tutorial.</td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2>Embedded videos</h2>
      <p>
        Videos in articles show only a still picture until you click play. Only then does the player
        load from YouTube (privacy-enhanced mode) or Vimeo, which may set their own cookies under
        their policies.
      </p>

      <h2>Your control</h2>
      <p>
        You can delete cookies and local storage at any time in your browser settings. The site
        still works, but you will be signed out and your preferences reset. More in our
        <a routerLink="/privacy">privacy policy</a>.
      </p>
    </main>
  `,
  styles: PROSE,
})
export class CookiesPage {
  protected readonly legal = LEGAL;
  constructor() {
    inject(Seo).describe({
      title: 'Cookie policy',
      description:
        'The cookies and browser storage ScoutPulse uses. All are necessary, and none track you.',
      path: '/cookies',
    });
  }
}
