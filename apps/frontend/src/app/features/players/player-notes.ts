import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  resource,
  signal,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';

import { PLAYER_NOTE_READER, PLAYER_NOTE_WRITER } from '../../core/api/contracts';
import { Permissions } from '../../core/auth/permissions';
import { SessionStore } from '../../core/auth/session-store';
import { PlayerNote } from '../../core/models/football';
import { messageFor } from '../../shared/forms/submit';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

/** Mirrors the server's cap, so the counter states the real limit. */
const MAX_NOTE_LENGTH = 4000;

/**
 * Member notes on a player.
 *
 * The rules here are deliberately unlike every other write in this app.
 * Editing a player needs an editor grant; **writing a note needs only an
 * account**, because the point is a range of opinions rather than one
 * authoritative record.
 *
 * What stops that becoming a spam column is that each person gets exactly one
 * note, which they can rewrite whenever they like. The constraint is in the
 * database — a unique index on (player, author) — not in this component, so two
 * tabs posting at once cannot both succeed.
 *
 * Reading is public. A signed-out visitor sees the discussion and is invited
 * to sign in rather than being shown an empty panel.
 */
@Component({
  selector: 'app-player-notes',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, DatePipe, Loading, Empty, ErrorState],
  template: `
    <section class="notes">
      <div class="section-head">
        <h4>Notes</h4>
        <span class="count muted">{{ total() }}</span>
      </div>

      @if (session.isAuthenticated()) {
        <form class="composer" (ngSubmit)="save()">
          <label class="visually-hidden" [attr.for]="'note-' + playerId()">
            {{ mine() ? 'Edit your note' : 'Write a note' }}
          </label>
          <textarea
            [id]="'note-' + playerId()"
            name="body"
            rows="4"
            [attr.maxlength]="maxLength"
            [placeholder]="mine() ? 'Edit your note…' : 'What did you make of this player?'"
            [(ngModel)]="draft"
          ></textarea>

          <div class="composer-foot">
            <!--
              Counts down rather than up, and only once it is worth knowing.
              A permanent "0 / 4000" on an empty box is noise.
            -->
            @if (remaining() < 500) {
              <span class="remaining" [class.over]="remaining() < 0">
                {{ remaining() }} characters left
              </span>
            }
            <span class="spacer"></span>
            @if (mine()) {
              <button class="btn danger" type="button" [disabled]="busy()" (click)="remove()">
                Delete
              </button>
            }
            <button class="btn primary" type="submit" [disabled]="busy() || !canSubmit()">
              {{ busy() ? 'Saving…' : mine() ? 'Save changes' : 'Post note' }}
            </button>
          </div>

          @if (error()) {
            <app-error-state [message]="error()!" />
          }
        </form>
      } @else {
        <p class="signed-out muted">
          <a routerLink="/login">Sign in</a> to add your own note. Everyone gets one, and you can
          edit yours whenever you like.
        </p>
      }

      @if (notes.isLoading()) {
        <app-loading [lines]="3" />
      } @else if (notes.error()) {
        <app-error-state [message]="listErrorMessage()" />
      } @else if (!notes.value()?.items?.length) {
        <app-empty message="No notes yet." hint="Be the first to write one." />
      } @else {
        <ul class="list">
          @for (note of notes.value()!.items; track note.id) {
            <li [class.own]="isMine(note)">
              <div class="byline">
                <span class="author">{{ note.author_name || 'A member' }}</span>
                @if (isMine(note)) {
                  <span class="you">you</span>
                }
                <time class="when muted" [attr.datetime]="note.created_at">
                  {{ note.created_at | date: 'd MMM y' }}
                </time>
                @if (note.updated_at !== note.created_at) {
                  <!--
                    Marked rather than hidden: a note that changed after people
                    replied to it should say so.
                  -->
                  <span class="edited muted">edited</span>
                }
                @if (permissions.canAdminister() && !isMine(note)) {
                  <button class="moderate" type="button" (click)="removeOther(note)">Remove</button>
                }
              </div>
              <p class="body">{{ note.body }}</p>
            </li>
          }
        </ul>
      }
    </section>
  `,
  styles: `
    .notes {
      margin-bottom: var(--space-7);
    }
    .section-head {
      display: flex;
      align-items: baseline;
      gap: var(--space-3);
      margin-bottom: var(--space-4);
    }
    h4 {
      margin: 0;
    }
    .count {
      font-size: var(--text-sm);
    }
    .composer {
      display: flex;
      flex-direction: column;
      gap: var(--space-3);
      margin-bottom: var(--space-5);
    }
    textarea {
      width: 100%;
      padding: var(--space-3);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
      color: var(--ink);
      font: inherit;
      resize: vertical;
    }
    .composer-foot {
      display: flex;
      align-items: center;
      gap: var(--space-2);
      flex-wrap: wrap;
    }
    .spacer {
      flex: 1;
    }
    .remaining {
      font-size: var(--text-xs);
      color: var(--muted);
    }
    .remaining.over {
      color: var(--critical);
    }
    .signed-out {
      padding: var(--space-4);
      border: 1px dashed var(--line);
      border-radius: var(--radius);
      margin-bottom: var(--space-5);
    }
    .list {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .list li {
      padding: var(--space-4) 0;
      border-bottom: 1px solid var(--line-soft);
    }
    .list li.own {
      border-left: 2px solid var(--accent);
      padding-left: var(--space-4);
    }
    .byline {
      display: flex;
      align-items: baseline;
      gap: var(--space-2);
      margin-bottom: var(--space-2);
      flex-wrap: wrap;
    }
    .author {
      font-weight: 600;
    }
    .you {
      font-family: var(--font-mono);
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--accent);
      border: 1px solid var(--accent);
      border-radius: var(--radius-sm);
      padding: 1px 5px;
    }
    .when,
    .edited {
      font-size: var(--text-xs);
    }
    .moderate {
      margin-left: auto;
      background: none;
      border: none;
      color: var(--critical);
      font-size: var(--text-xs);
      cursor: pointer;
      padding: 0;
    }
    .body {
      /* Notes are typed with line breaks and they should survive. */
      white-space: pre-wrap;
      max-width: var(--measure);
    }
    .muted {
      color: var(--muted);
    }
  `,
})
export class PlayerNotes {
  private readonly reader = inject(PLAYER_NOTE_READER);
  private readonly writer = inject(PLAYER_NOTE_WRITER);
  protected readonly session = inject(SessionStore);
  protected readonly permissions = inject(Permissions);

  readonly playerId = input.required<string>();

  protected readonly maxLength = MAX_NOTE_LENGTH;
  protected readonly draft = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);

  protected readonly notes = resource({
    params: () => ({ id: this.playerId() }),
    loader: ({ params }) => this.reader.notes(params.id, { limit: 100 }),
  });

  /**
   * The caller's own note, loaded separately from the list.
   *
   * Separately because the list is paged: on a player with two hundred notes,
   * yours might not be on the first page, and the composer would then offer to
   * create a second one that the API would refuse.
   */
  protected readonly mineResource = resource({
    params: () => {
      // Idle for signed-out visitors — the endpoint needs a token, and asking
      // without one is a guaranteed 401 in the console.
      return this.session.isAuthenticated() ? { id: this.playerId() } : undefined;
    },
    loader: async ({ params }) => {
      const existing = await this.reader.myNote(params.id);
      if (existing) this.draft.set(existing.body);
      return existing;
    },
  });

  protected readonly mine = computed(() => this.mineResource.value() ?? null);

  protected readonly total = computed(() => {
    const count = this.notes.value()?.items.length ?? 0;
    return count === 1 ? '1 note' : `${count} notes`;
  });

  protected readonly remaining = computed(() => MAX_NOTE_LENGTH - this.draft().length);

  protected readonly canSubmit = computed(
    () => this.draft().trim().length > 0 && this.remaining() >= 0,
  );

  protected readonly listErrorMessage = computed(() =>
    messageFor(this.notes.error(), 'Could not load the notes.'),
  );

  protected isMine(note: PlayerNote): boolean {
    return note.author_id === this.session.user()?.id;
  }

  protected async save(): Promise<void> {
    if (this.busy() || !this.canSubmit()) return;

    this.busy.set(true);
    this.error.set(null);

    try {
      const existing = this.mine();
      if (existing) {
        await this.writer.edit(this.playerId(), existing.id, this.draft());
      } else {
        await this.writer.write(this.playerId(), this.draft());
      }
      this.notes.reload();
      this.mineResource.reload();
    } catch (error) {
      this.error.set(messageFor(error, 'Could not save your note.'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async remove(): Promise<void> {
    const existing = this.mine();
    if (!existing || this.busy()) return;
    if (!confirm('Delete your note on this player?')) return;

    this.busy.set(true);
    this.error.set(null);

    try {
      await this.writer.removeNote(this.playerId(), existing.id);
      this.draft.set('');
      this.notes.reload();
      this.mineResource.reload();
    } catch (error) {
      this.error.set(messageFor(error, 'Could not delete your note.'));
    } finally {
      this.busy.set(false);
    }
  }

  /** Moderation. Administrators only; the API enforces the same rule. */
  protected async removeOther(note: PlayerNote): Promise<void> {
    if (!confirm(`Remove ${note.author_name || 'this member'}'s note?`)) return;

    try {
      await this.writer.removeNote(this.playerId(), note.id);
      this.notes.reload();
    } catch (error) {
      this.error.set(messageFor(error, 'Could not remove that note.'));
    }
  }
}
