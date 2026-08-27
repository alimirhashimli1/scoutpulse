import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';

import { AuthFacade } from '../core/auth/auth-facade';
import { SessionStore } from '../core/auth/session-store';
import { ThemeStore } from '../core/theme/theme';

/**
 * The frame every page renders inside: masthead, navigation, search, footer.
 *
 * Navigation deliberately offers no standings or fixtures. There is no match
 * data, and a nav item leading to an empty page is worse than its absence.
 */
@Component({
  selector: 'app-shell',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterOutlet, RouterLink, RouterLinkActive, FormsModule],
  template: `
    <a class="skip visually-hidden" href="#main">Skip to content</a>

    <header class="masthead">
      <div class="bar page">
        <!--
          The badge sits outside the link on purpose, so the link's accessible
          name stays "ScoutPulse" rather than becoming "ScoutPulse Beta" — the
          destination has not changed, only the disclaimer beside it.
        -->
        <div class="brand">
          <a class="wordmark" routerLink="/">ScoutPulse</a>
          <span class="beta">Beta</span>
        </div>

        <form class="search" role="search" (ngSubmit)="search()">
          <label class="visually-hidden" for="q">Search players, clubs and competitions</label>
          <input
            id="q"
            name="q"
            type="search"
            placeholder="Search players, clubs, coaches…"
            [(ngModel)]="query"
          />
        </form>

        <nav class="links">
          <a routerLink="/transfers" routerLinkActive="active">Transfers</a>
          <a routerLink="/clubs" routerLinkActive="active">Clubs</a>
          <a routerLink="/competitions" routerLinkActive="active">Competitions</a>
        </nav>

        <div class="account">
          <!--
            Cycles system → light → dark. aria-label carries the destination,
            because the glyph alone tells a screen reader nothing, and the
            label changing is what announces that the press did something.
          -->
          <button
            type="button"
            class="theme"
            [attr.aria-label]="theme.nextLabel()"
            [title]="theme.nextLabel()"
            (click)="theme.cycle()"
          >
            <span aria-hidden="true">{{ theme.icon() }}</span>
          </button>

          @if (session.isAuthenticated()) {
            @if (session.isAdmin()) {
              <a routerLink="/admin/users" routerLinkActive="active">Users</a>
            }
            <a class="who" routerLink="/account" routerLinkActive="active">
              {{ session.user()?.username }}
              @if (session.role(); as role) {
                @if (role !== 'user') {
                  <span class="role">{{ role }}</span>
                }
              }
            </a>
            <button type="button" (click)="signOut()">Sign out</button>
          } @else if (session.status() === 'anonymous') {
            <a routerLink="/login">Sign in</a>
          }
          <!--
            While status is 'unknown' the session restore has not finished.
            Rendering "Sign in" then swapping it for a username is a flash of
            the wrong state on every reload, so nothing renders until we know.
          -->
        </div>
      </div>
    </header>

    <div id="main">
      <router-outlet />
    </div>

    <footer class="footer">
      <div class="page">
        <p>
          ScoutPulse — transfers, valuations and careers over time.
          <span class="muted">No match data: this is a record of people and clubs.</span>
        </p>
        <!--
          The badge says "beta"; this says what beta means here. A label on its
          own tells someone not to trust the site without telling them which
          part to distrust.
        -->
        <p class="muted beta-note">
          Beta — the data is still being filled in and things may change.
        </p>
      </div>
    </footer>
  `,
  styles: `
    .skip:focus {
      position: fixed;
      top: var(--space-3);
      left: var(--space-3);
      width: auto;
      height: auto;
      clip-path: none;
      background: var(--surface);
      padding: var(--space-3);
      border: 1px solid var(--accent);
      border-radius: var(--radius);
      z-index: 10;
    }

    .masthead {
      border-bottom: 1px solid var(--line);
      background: var(--surface);
      position: sticky;
      top: 0;
      z-index: 5;
    }
    .bar {
      display: flex;
      align-items: center;
      gap: var(--space-5);
      padding-block: var(--space-3);
      flex-wrap: wrap;
    }
    .brand {
      display: flex;
      flex-direction: column;
      align-items: flex-start;
      /* Tight: the badge is a caption on the wordmark, not a second line of
         navigation, and the masthead is only so tall. */
      gap: 1px;
      line-height: 1.1;
    }
    .wordmark {
      font-family: var(--font-display);
      font-size: var(--text-lg);
      color: var(--ink);
      text-decoration: none;
      letter-spacing: -0.01em;
    }
    .beta {
      font-family: var(--font-mono);
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.14em;
      color: var(--accent);
    }
    .beta-note {
      margin-top: var(--space-2);
      font-size: var(--text-xs);
    }
    .search {
      flex: 1 1 14rem;
      min-width: 0;
    }
    .search input {
      width: 100%;
      padding: var(--space-2) var(--space-3);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--ground);
    }
    .links {
      display: flex;
      gap: var(--space-4);
    }
    .links a {
      color: var(--ink-soft);
      text-decoration: none;
      font-size: var(--text-sm);
    }
    .links a:hover,
    .links a.active {
      color: var(--accent);
    }
    .account {
      display: flex;
      align-items: center;
      gap: var(--space-3);
      font-size: var(--text-sm);
    }
    .account a {
      color: var(--ink-soft);
      text-decoration: none;
    }
    .account a:hover,
    .account a.active {
      color: var(--accent);
    }
    .who {
      display: flex;
      align-items: center;
      gap: var(--space-2);
    }
    .role {
      font-family: var(--font-mono);
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--accent);
      background: var(--accent-soft);
      padding: 2px 6px;
      border-radius: var(--radius-sm);
    }
    .account button {
      background: none;
      border: 1px solid var(--line);
      border-radius: var(--radius);
      padding: var(--space-1) var(--space-3);
      cursor: pointer;
      color: var(--ink-soft);
    }
    .account button:hover {
      border-color: var(--accent);
      color: var(--accent);
    }
    .theme {
      /* Square, so the three glyphs do not resize the masthead as it cycles. */
      width: 2rem;
      height: 2rem;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      padding: 0;
      line-height: 1;
    }

    .footer {
      border-top: 1px solid var(--line);
      margin-top: var(--space-8);
      padding-block: var(--space-5);
      font-size: var(--text-sm);
      color: var(--ink-soft);
    }
    .muted {
      color: var(--muted);
    }

    @media (max-width: 40rem) {
      .bar {
        gap: var(--space-3);
      }
      .search {
        order: 3;
        flex-basis: 100%;
      }
    }
  `,
})
export class Shell {
  protected readonly session = inject(SessionStore);
  protected readonly theme = inject(ThemeStore);
  private readonly auth = inject(AuthFacade);
  private readonly router = inject(Router);

  protected readonly query = signal('');

  protected async search(): Promise<void> {
    const q = this.query().trim();
    // The API rejects anything shorter than two characters, so there is no
    // point navigating to a results page that can only show an error.
    if (q.length < 2) return;
    await this.router.navigate(['/search'], { queryParams: { q } });
  }

  protected async signOut(): Promise<void> {
    await this.auth.logout();
    await this.router.navigateByUrl('/');
  }
}
