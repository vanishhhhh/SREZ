import { useState } from 'react';
import { api } from '../../lib/api';
import type { ManagedImage } from '../../lib/types';
import { UploadField } from '../media/UploadField';
import { Confirm, Icon, Media, useAction } from '../../components/ui';

function ImageTile({
  image,
  images,
  index,
  productID,
  colorID,
  changed,
}: {
  image: ManagedImage;
  images: ManagedImage[];
  index: number;
  productID: number;
  colorID: number;
  changed: () => Promise<void>;
}) {
  const [editing, setEditing] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const action = useAction();
  function move(delta: number) {
    void action.run(async () => {
      const ids = images.map((i) => i.id);
      [ids[index], ids[index + delta]] = [ids[index + delta], ids[index]];
      await api.reorderImages(productID, colorID, ids);
      await changed();
    }, 'Порядок фотографий изменён');
  }
  return (
    <div className="editor-image">
      <div className="editor-image__visual">
        <Media authenticated src={image.url} alt={'Фото ' + (index + 1)} />
        <span className="editor-image__label">
          {index === 0 ? 'ОБЛОЖКА' : String(index + 1).padStart(2, '0')}
        </span>
      </div>
      <div className="editor-image__actions">
        <button
          aria-label={`Переместить фото ${index + 1} раньше`}
          disabled={index === 0 || action.busy}
          onClick={() => move(-1)}
        >
          <Icon name="back" size={16} />
        </button>
        <button
          aria-label={`Переместить фото ${index + 1} позже`}
          disabled={index === images.length - 1 || action.busy}
          onClick={() => move(1)}
        >
          <Icon name="arrow" size={16} />
        </button>
        <button
          aria-label={`Заменить фото ${index + 1}`}
          disabled={action.busy}
          onClick={() => setEditing(!editing)}
        >
          Заменить
        </button>
        <button
          aria-label={`Удалить фото ${index + 1}`}
          disabled={action.busy}
          onClick={() => setConfirm(true)}
        >
          <Icon name="trash" size={15} />
        </button>
      </div>
      {index > 0 && (
        <button
          className="image-cover-button"
          disabled={action.busy}
          onClick={() =>
            void action.run(async () => {
              await api.patchImage(productID, colorID, image.id, { position: 0 });
              await changed();
            }, 'Обложка изменена')
          }
        >
          Сделать обложкой
        </button>
      )}
      {editing && (
        <UploadField
          label={'Заменить фото ' + (index + 1)}
          target={{
            purpose: 'product_image',
            product_id: productID,
            color_id: colorID,
            image_id: image.id,
          }}
          onComplete={async () => {
            await changed();
            setEditing(false);
          }}
          disabled={action.busy}
        />
      )}
      <Confirm
        open={confirm}
        title="Удалить фотографию?"
        busy={action.busy}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          void action.run(async () => {
            await api.deleteImage(productID, colorID, image.id);
            setConfirm(false);
            await changed();
          }, 'Фотография удалена')
        }
      >
        У опубликованного цвета должна остаться хотя бы одна фотография.
      </Confirm>
    </div>
  );
}
export function Images({
  images,
  productID,
  colorID,
  changed,
}: {
  images: ManagedImage[];
  productID: number;
  colorID: number;
  changed: () => Promise<void>;
}) {
  return (
    <section className="editor-subsection" id="photos">
      <div className="panel-heading">
        <h3>Фотографии</h3>
        <span className="mono">{images.length} / 10</span>
      </div>
      <p className="muted">
        Первая фотография станет обложкой этого цвета. Меняйте порядок стрелками.
      </p>
      {images.length > 0 && (
        <div className="editor-image-grid">
          {images.map((image, index) => (
            <ImageTile
              key={image.id}
              image={image}
              images={images}
              index={index}
              productID={productID}
              colorID={colorID}
              changed={changed}
            />
          ))}
        </div>
      )}
      <div className="image-add">
        <UploadField
          label="Добавить фотографию"
          target={{ purpose: 'product_image', product_id: productID, color_id: colorID }}
          onComplete={changed}
          disabled={images.length >= 10}
        />
      </div>
    </section>
  );
}
