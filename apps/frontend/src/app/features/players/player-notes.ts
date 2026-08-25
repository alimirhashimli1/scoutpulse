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
import { ConfirmDialog } from '../../shared/ui/confirm-dialog';
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
 * **Your own note appears once.** An earlier version showed it in the list
 * *and* pre-filled the composer with it, so the same text sat on the page
 * twice with a permanently-open edit box under it. Now the composer only
 * exists when you have nothing to say yet; once you do, your note is pinned to
 * the top of the list with Edit and Delete on it, and editing happens in place.
 *
 * Reading is public. A signed-out visitor sees the discussion and is invited
 * to sign in rather than being shown an empty panel.
 */
@Component({
  selector: 'app-player-notes',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, DatePipe, ConfirmDialog, Loading, Empty, ErrorState],
  template: `
    <section class="notes">
      <div class="section-head">
        <h4>Notes</h4>
        <span class="count muted">{{ total() }}</span>
      </div>

      @if (error()) {
        <app-error-state [message]="error()!" />
      }

      <!-- Compose: only when this member has not written one yet. -->
      @if (session.isAuthenticated() && !mine() && !mineResource.isLoading()) {
        <form class="composer" (ngSubmit)="post()">
          <label class="visually-hidden" [attr.for]="'note-' + playerId()">Write a note</label>
          <textarea
            [id]="'note-' + playerId()"
            name="body"
            rows="4"
            [attr.maxlength]="maxLength"
            placeholder="What did you make of this player?"
            [(ngModel)]="draft"
          ></textarea>

          <div class="composer-foot">
            @if (remaining(draft()) < 500) {
              <span class="remaining" [class.over]="remaining(draft()) < 0">
                {{ remaining(draft()) }} characters left
              </span>
            }
            <span class="spacer"></span>
            <button class="btn primary" type="submit" [disabled]="busy() || !valid(draft())">
              {{ busy() ? 'Posting…' : 'Post note' }}
            </button>
          </div>
        </form>
      } @else if (!session.isAuthenticated()) {
        <p class="signed-out muted">
          <a routerLink="/login">Sign in</a> to add your own note. Everyone gets one, and you can
          edit yours whenever you like.
        </p>
      }

      @if (notes.isLoading()) {
        <app-loading [lines]="3" />
      } @else if (notes.error()) {
        <app-error-state [message]="listErrorMessage()" />
      } @else if (!ordered().length) {
        <app-empty message="No notes yet." hint="Be the first to write one." />
      } @else {
        <ul class="list">
          @for (note of ordered(); track note.id) {
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
                    read it should say so.
                  -->
                  <span class="edited muted">edited</span>
                }

                <span class="spacer"></span>

                @if (isMine(note) && editingId() !== note.id) {
                  <button class="link-btn" type="button" (click)="startEdit(note)">Edit</button>
                  <button class="link-btn danger" type="button" (click)="askDelete(note)">
                    Delete
                  </button>
                } @else if (permissions.canAdminister() && !isMine(note)) {
                  <button class="link-btn danger" type="button" (click)="askDelete(note)">
                    Remove
                  </button>
                }
              </div>

              @if (editingId() === note.id) {
                <!-- Editing happens where the note is, not in a second box. -->
                <form class="composer inline" (ngSubmit)="saveEdit(note)">
                  <label class="visually-hidden" [attr.for]="'edit-' + note.id">
                    Edit your note
                  </label>
                  <textarea
                    [id]="'edit-' + note.id"
                    name="editBody"
                    rows="4"
                    [attr.maxlength]="maxLength"
                    [(ngModel)]="editDraft"
                  ></textarea>
                  <div class="composer-foot">
                    @if (remaining(editDraft()) < 500) {
                      <span class="remaining" [class.over]="remaining(editDraft()) < 0">
                        {{ remaining(editDraft()) }} characters left
                      </span>
                    }
                    <span class="spacer"></span>
                    <button class="btn" type="button" (click)="cancelEdit()">Cancel</button>
                    <button
                      class="btn primary"
                      type="submit"
                      [disabled]="busy() || !valid(editDraft())"
                    >
                      {{ busy() ? 'Saving…' : 'Save changes' }}
                    </button>
                  </div>
                </form>
              } @else {
                <p class="body">{{ note.body }}</p>
              }
            </li>
          }
        </ul>
      }

      <app-confirm-dialog
        [(open)]="confirmingDelete"
        [title]="deleteTitle()"
        [message]="deleteMessage()"
        confirmLabel="Delete"
        [danger]="true"
        (confirmed)="confirmDelete()"
      />
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
    .composer.inline {
      margin-bottom: 0;
      margin-top: var(--space-2);
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
      background: var(--accent-soft);
      border-radius: var(--radius);
      padding-right: var(--space-4);
      margin-bottom: var(--space-3);
      border-bottom: none;
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
    .link-btn {
      background: none;
      border: none;
      padding: 0;
      cursor: pointer;
      font-size: var(--text-xs);
      color: var(--accent);
      text-decoration: underline;
      text-underline-offset: 3px;
    }
    .link-btn.danger {
      color: var(--critical);
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
  protected readonly editDraft = signal('');
  /** The note currently open for editing, if any. */
  protected readonly editingId = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);

  protected readonly confirmingDelete = signal(false);
  private readonly pendingDelete = signal<PlayerNote | null>(null);

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
    loader: ({ params }) => this.reader.myNote(params.id),
  });

  protected readonly mine = computed(() => this.mineResource.value() ?? null);

  /**
   * The list with your own note first.
   *
   * Pinned rather than left in date order so it is somewhere predictable to
   * edit, and de-duplicated against the list: `mine` is fetched separately, so
   * on the first page it is usually present in both and would otherwise render
   * twice.
   */
  protected readonly ordered = computed<PlayerNote[]>(() => {
    const all = this.notes.value()?.items ?? [];
    const own = this.mine();
    if (!own) return all;

    return [own, ...all.filter((note) => note.id !== own.id)];
  });

  protected readonly total = computed(() => {
    const count = this.ordered().length;
    return count === 1 ? '1 note' : `${count} notes`;
  });

  protected readonly listErrorMessage = computed(() =>
    messageFor(this.notes.error(), 'Could not load the notes.'),
  );

  protected readonly deleteTitle = computed(() => {
    const note = this.pendingDelete();
    if (note && !this.isMine(note)) return 'Remove this note?';
    return 'Delete your note?';
  });

  protected readonly deleteMessage = computed(() => {
    const note = this.pendingDelete();
    if (!note) return '';
    if (!this.isMine(note)) {
      return `${note.author_name || 'This member'}'s note will be removed. This cannot be undone.`;
    }
    return 'Your note will be removed from this player. You can write a new one afterwards.';
  });

  protected remaining(text: string): number {
    return MAX_NOTE_LENGTH - text.length;
  }

  protected valid(text: string): boolean {
    return text.trim().length > 0 && this.remaining(text) >= 0;
  }

  protected isMine(note: PlayerNote): boolean {
    return note.author_id === this.session.user()?.id;
  }

  protected startEdit(note: PlayerNote): void {
    this.editDraft.set(note.body);
    this.editingId.set(note.id);
    this.error.set(null);
  }

  protected cancelEdit(): void {
    this.editingId.set(null);
    this.editDraft.set('');
  }

  protected async post(): Promise<void> {
    if (this.busy() || !this.valid(this.draft())) return;

    this.busy.set(true);
    this.error.set(null);

    try {
      await this.writer.write(this.playerId(), this.draft());
      this.draft.set('');
      this.refresh();
    } catch (error) {
      this.error.set(messageFor(error, 'Could not post your note.'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async saveEdit(note: PlayerNote): Promise<void> {
    if (this.busy() || !this.valid(this.editDraft())) return;

    this.busy.set(true);
    this.error.set(null);

    try {
      await this.writer.edit(this.playerId(), note.id, this.editDraft());
      this.cancelEdit();
      this.refresh();
    } catch (error) {
      this.error.set(messageFor(error, 'Could not save your note.'));
    } finally {
      this.busy.set(false);
    }
  }

  /** Opens the dialog. Nothing is deleted until it is confirmed. */
  protected askDelete(note: PlayerNote): void {
    this.pendingDelete.set(note);
    this.confirmingDelete.set(true);
  }

  protected async confirmDelete(): Promise<void> {
    const note = this.pendingDelete();
    if (!note) return;

    this.error.set(null);

    try {
      await this.writer.removeNote(this.playerId(), note.id);
      if (this.isMine(note)) this.draft.set('');
      this.cancelEdit();
      this.refresh();
    } catch (error) {
      this.error.set(messageFor(error, 'Could not delete that note.'));
    } finally {
      this.pendingDelete.set(null);
    }
  }

  /** Both resources, because a write changes the list and your own note. */
  private refresh(): void {
    this.notes.reload();
    this.mineResource.reload();
  }
}
